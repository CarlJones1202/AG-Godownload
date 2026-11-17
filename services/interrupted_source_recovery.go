package services

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type GallerySourceAudit struct {
	GalleryID      uint
	GalleryName    string
	SourceID       uint
	SourceLocation string
	GalleryPhotos  int64
	SourcePhotos   int
	SourceRequeued bool
	SourcePending  bool
}

type GallerySourceAuditFailure struct {
	GalleryID uint
	Err       error
}

type GallerySourceAuditBatch struct {
	Audits   []*GallerySourceAudit
	Skipped  int
	Failures []GallerySourceAuditFailure
}

// AuditGallerySource compares saved photos with the source post and marks
// mismatched sources queued so they remain recoverable if live queueing fails.
func AuditGallerySource(galleryID uint) (*GallerySourceAudit, error) {
	var gallery models.Gallery
	if err := database.DB.First(&gallery, galleryID).Error; err != nil {
		return nil, fmt.Errorf("load gallery %d: %w", galleryID, err)
	}
	if gallery.SourceID == nil {
		return nil, fmt.Errorf("gallery %d has no source", galleryID)
	}

	var source models.Source
	if err := database.DB.First(&source, *gallery.SourceID).Error; err != nil {
		return nil, fmt.Errorf("load source %d for gallery %d: %w", *gallery.SourceID, galleryID, err)
	}
	parsedURL, err := url.Parse(source.Location)
	if !isHTTPSourceURL(parsedURL, err) {
		return nil, fmt.Errorf("source %d location is not an HTTP(S) URL", source.ID)
	}

	report := &GallerySourceAudit{
		GalleryID:      gallery.ID,
		GalleryName:    gallery.Name,
		SourceID:       source.ID,
		SourceLocation: source.Location,
	}
	if err := database.DB.Model(&models.Image{}).
		Joins("LEFT JOIN image_galleries ON image_galleries.image_id = images.id").
		Where("(image_galleries.gallery_id = ? OR images.gallery_id = ?) AND images.type IN ?",
			gallery.ID, gallery.ID, []string{"image", ""}).
		Distinct("images.id").
		Count(&report.GalleryPhotos).Error; err != nil {
		return nil, fmt.Errorf("count gallery %d photos: %w", galleryID, err)
	}

	req, err := http.NewRequest(http.MethodGet, source.Location, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for source %d: %w", source.ID, err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/117.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", source.Location)
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("DNT", "1")

	resp, err := GetHTTPClient(source.Location).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch source %d: %w", source.ID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch source %d: unexpected HTTP status %d", source.ID, resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse source %d page: %w", source.ID, err)
	}
	selection := selectSourcePost(doc, source.Location, parsedURL.Fragment)
	if selection.Length() == 0 {
		return nil, fmt.Errorf("no post content found for source %d", source.ID)
	}
	report.SourcePhotos = countPostImageLinks(selection)

	if report.GalleryPhotos != int64(report.SourcePhotos) {
		result := database.DB.Model(&source).
			Where("status IS NULL OR status NOT IN ?", []string{"queued", "crawling"}).
			Update("status", "queued")
		if result.Error != nil {
			return nil, fmt.Errorf("queue source %d: %w", source.ID, result.Error)
		}
		if result.RowsAffected == 0 {
			report.SourcePending = true
			return report, nil
		}
		report.SourceRequeued = true
	}
	return report, nil
}

// AuditAllGallerySources audits crawlable galleries sequentially. Galleries
// without an HTTP(S) source are skipped, and individual failures do not stop
// the remaining audits.
func AuditAllGallerySources() (*GallerySourceAuditBatch, error) {
	var galleries []struct {
		ID             uint
		SourceID       *uint
		SourceLocation string
	}
	if err := database.DB.Model(&models.Gallery{}).
		Select("galleries.id, galleries.source_id, sources.location AS source_location").
		Joins("LEFT JOIN sources ON sources.id = galleries.source_id AND sources.deleted_at IS NULL").
		Order("galleries.id ASC").
		Find(&galleries).Error; err != nil {
		return nil, fmt.Errorf("list galleries for source audit: %w", err)
	}

	batch := &GallerySourceAuditBatch{}
	for _, gallery := range galleries {
		parsedURL, err := url.Parse(gallery.SourceLocation)
		if gallery.SourceID == nil || !isHTTPSourceURL(parsedURL, err) {
			batch.Skipped++
			continue
		}

		audit, err := AuditGallerySource(gallery.ID)
		if err != nil {
			batch.Failures = append(batch.Failures, GallerySourceAuditFailure{
				GalleryID: gallery.ID,
				Err:       err,
			})
			continue
		}
		batch.Audits = append(batch.Audits, audit)
	}
	return batch, nil
}

func isHTTPSourceURL(parsedURL *url.URL, err error) bool {
	return err == nil && parsedURL != nil && parsedURL.Host != "" &&
		(parsedURL.Scheme == "http" || parsedURL.Scheme == "https")
}

func selectSourcePost(doc *goquery.Document, sourceLocation, fragment string) *goquery.Selection {
	if strings.HasPrefix(fragment, "post") {
		postID := strings.TrimPrefix(fragment, "post")
		logger.Debugf("Crawling specific post ID: %s", postID)
		return doc.Find(fmt.Sprintf("div[id='post_message_%s']", postID))
	}
	if strings.Contains(sourceLocation, "jkforum.net") {
		logger.Debug("Crawling jkforum.net")
		selection := doc.Find("article, div.article-content, div.post-content, div[id^='post_message_']")
		if selection.Length() == 0 {
			logger.Debug("No specific post container found, falling back to body")
			return doc.Find("body")
		}
		return selection
	}
	if strings.Contains(sourceLocation, "kitty-kats.net") {
		logger.Debug("Crawling kitty-kats.net")
		selection := doc.Find("article.message--post .message-userContent .bbWrapper")
		if selection.Length() == 0 {
			logger.Debug("No kitty-kats specific container found, falling back to first post")
			return doc.Find("div[id^='post_message_']").First()
		}
		return selection
	}
	logger.Debug("Crawling first post only")
	return doc.Find("div[id^='post_message_']").First()
}

func countPostImageLinks(selection *goquery.Selection) int {
	count := 0
	selection.Each(func(_ int, post *goquery.Selection) {
		post.Find("a img").Each(func(_ int, img *goquery.Selection) {
			if img.AttrOr("alt", "") != "View Post" {
				count++
			}
		})
	})
	return count
}
