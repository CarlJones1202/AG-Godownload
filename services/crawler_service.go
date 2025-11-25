package services

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

func CrawlSource(sourceID uint) error {
	var source models.Source
	if err := database.DB.First(&source, sourceID).Error; err != nil {
		return err
	}

	// Update status to crawling
	database.DB.Model(&source).Updates(models.Source{Status: "crawling", LastCheckedAt: time.Now()})

	// Find or create gallery for this source
	var gallery models.Gallery
	if err := database.DB.Where("source_id = ?", source.ID).First(&gallery).Error; err != nil {
		// Create if not exists
		gallery = models.Gallery{
			Name:     source.Name,
			SourceID: &source.ID,
		}
		if err := database.DB.Create(&gallery).Error; err != nil {
			database.DB.Model(&source).Update("Status", "error")
			return err
		}
	}

	// Fetch the URL
	resp, err := http.Get(source.Location)
	if err != nil {
		database.DB.Model(&source).Update("Status", "error")
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		database.DB.Model(&source).Update("Status", "error")
		return fmt.Errorf("failed to fetch source: status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		database.DB.Model(&source).Update("Status", "error")
		return err
	}

	// Logic from reference: find div[id^='post_message_']
	doc.Find("div[id^='post_message_']").Each(func(i int, s *goquery.Selection) {
		// Find images inside this div
		s.Find("img").Each(func(j int, img *goquery.Selection) {
			src, exists := img.Attr("src")
			if !exists {
				return
			}

			// Check if it's a thumbnail linking to a larger image (common in forums)
			// The reference code checks parent <a> tag
			parent := img.Parent()
			if parent.Is("a") {
				href, hrefExists := parent.Attr("href")
				if hrefExists {
					// Use the href as the image URL if it looks like an image or a known host
					// For simplicity, we'll try to download the href if it ends in an image extension
					// or if we implement specific logic later.
					// For now, let's just use the href if it looks like an image, otherwise fallback to src
					lowerHref := strings.ToLower(href)
					if strings.HasSuffix(lowerHref, ".jpg") || strings.HasSuffix(lowerHref, ".png") || strings.HasSuffix(lowerHref, ".jpeg") {
						src = href
					}
				}
			}

			// Basic deduplication check (by URL)
			var count int64
			database.DB.Model(&models.Image{}).Where("original_url = ? AND gallery_id = ?", src, gallery.ID).Count(&count)
			if count > 0 {
				return
			}

			// Download
			filename := filepath.Base(src)
			// Sanitize filename
			filename = strings.Split(filename, "?")[0]

			destPath, err := DownloadImage(src, filename)
			if err != nil {
				fmt.Printf("Failed to download %s: %v\n", src, err)
				return
			}

			// Generate thumbnail
			_, err = GenerateThumbnail(destPath)
			if err != nil {
				fmt.Printf("Failed to generate thumbnail for %s: %v\n", filename, err)
			}

			// Save to DB
			image := models.Image{
				GalleryID:   gallery.ID,
				Filename:    filepath.Base(destPath),
				OriginalURL: src,
			}
			database.DB.Create(&image)
		})
	})

	database.DB.Model(&source).Update("Status", "idle")
	return nil
}
