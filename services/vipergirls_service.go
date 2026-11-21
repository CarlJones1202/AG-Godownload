package services

import (
	"context"
	"encoding/json"
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	ForumSiteVipergirls = "vipergirls"
	ForumSiteKittyKats  = "kitty-kats"

	vipergirlsBaseURL      = "https://vipergirls.to"
	vipergirlsMaxResults   = 50
	vipergirlsMaxQueryLen  = 200
	vipergirlsMaxBodySize  = 10 << 20 // 10 MiB
	vipergirlsUserAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	vipergirlsSearchTimeout = 20 * time.Second
	kittyKatsSearchTimeout  = 25 * time.Second
)

var (
	vipergirlsThreadPathRE = regexp.MustCompile(`^/threads/(\d+)(?:-|/|$)`)
	videoForumRE = regexp.MustCompile(`(?i)\b(video|videos|movie|movies|clip|clips|film|films)\b`)

	// These are the forum IDs the site itself submits when "Search photo sections"
	// is checked in its quick-search form. Sending imgsearch=1 together with these
	// IDs is what actually restricts results to photo forums; forumchoice alone is
	// ignored by the custom search code.
	vipergirlsPhotoForumIDs = []string{"25", "161", "233", "235", "268", "372", "274"}
)

// ForumSearchResult is one thread from a supported forum search. The JSON shape is
// shared by every source so the UI can render them in one grid.
type ForumSearchResult struct {
	Site         string `json:"site"`
	ThreadID     string `json:"thread_id"`
	Title        string `json:"title"`
	SubForum     string `json:"sub_forum"`
	ThreadURL    string `json:"thread_url"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
}

// VipergirlsSearchResult is kept as an alias for existing callers.
type VipergirlsSearchResult = ForumSearchResult

// ForumSearchSource describes one searched site, including a site-specific failure.
// A partial failure is reported here instead of discarding results from other sites.
type ForumSearchSource struct {
	Site      string `json:"site"`
	Label     string `json:"label"`
	SearchURL string `json:"search_url"`
	Error     string `json:"error,omitempty"`
}

// VipergirlsSearchResponse is the payload returned to the search popup. The endpoint
// name and JSON shape are retained for existing callers; the response now aggregates
// every supported forum.
type VipergirlsSearchResponse struct {
	Query     string                  `json:"query"`
	SearchURL string                  `json:"search_url"`
	Sources   []ForumSearchSource     `json:"sources"`
	Data      []VipergirlsSearchResult `json:"data"`
}

// ScrapeVipergirlsRequest carries the missing-gallery context alongside the chosen thread.
type ScrapeVipergirlsRequest struct {
	PersonID     uint   `json:"person_id" binding:"required"`
	Provider     string `json:"provider" binding:"required"`
	GalleryName  string `json:"gallery_name" binding:"required"`
	MissingURL   string `json:"missing_url,omitempty"`
	Title        string `json:"title,omitempty"`
	ThreadURL    string `json:"thread_url" binding:"required"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
}

// ScrapeVipergirlsResponse reports what was created for a scrape request.
type ScrapeVipergirlsResponse struct {
	GalleryID uint   `json:"gallery_id"`
	SourceID  uint   `json:"source_id"`
	Queued    bool   `json:"queued"`
	Message   string `json:"message"`
}

// VipergirlsSearchURL builds the same photo-section, title-only search the site's
// own quick-search form produces when "Search photo sections" is checked.
func VipergirlsSearchURL(query string) string {
	params := url.Values{}
	params.Set("contenttype", "1")
	params.Set("do", "process")
	params.Set("query", query)
	params.Set("titleonly", "1")
	params.Set("imgsearch", "1")
	params.Set("childforums", "1")
	for _, forumID := range vipergirlsPhotoForumIDs {
		params.Add("forumchoice[]", forumID)
	}
	return vipergirlsBaseURL + "/search.php?" + params.Encode()
}

// IsVipergirlsThreadURL accepts only canonical https vipergirls.to thread URLs.
func IsVipergirlsThreadURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(u.Scheme, "https") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host != "vipergirls.to" && host != "www.vipergirls.to" {
		return false
	}
	return vipergirlsThreadPathRE.MatchString(u.Path)
}

// IsForumThreadURL accepts a thread from any supported forum search source.
func IsForumThreadURL(raw string) bool {
	return IsVipergirlsThreadURL(raw) || IsKittyKatsThreadURL(raw)
}

// isPhotoForum keeps only results with an identifiable, non-video sub forum.
// Forum names use "Videos"/"Movies"/"Clips"/"Films" for video sections
// (e.g. "Hardcore Videos", "Adult Movies"), so those are excluded here.
func isPhotoForum(subForum string) bool {
	subForum = strings.TrimSpace(subForum)
	if subForum == "" || videoForumRE.MatchString(subForum) {
		return false
	}
	return true
}

// isVipergirlsPhotoForum is retained for existing tests and callers.
func isVipergirlsPhotoForum(subForum string) bool { return isPhotoForum(subForum) }

func isAbsoluteHTTPURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// isVipergirlsImageURL is retained for existing callers.
func isVipergirlsImageURL(raw string) bool { return isAbsoluteHTTPURL(raw) }

func vipergirlsThreadID(threadURL string) string {
	u, err := url.Parse(threadURL)
	if err != nil {
		return ""
	}
	matches := vipergirlsThreadPathRE.FindStringSubmatch(u.Path)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

func canonicalVipergirlsURL(base *url.URL, raw string) (string, error) {
	ref, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	u := base.ResolveReference(ref)
	// Search links carry a per-request session id and a highlight parameter; neither
	// belongs in a stored source URL.
	u.RawQuery = ""
	u.Fragment = ""
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
	return u.String(), nil
}

func looksLikeVipergirlsThumbnail(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	path := strings.ToLower(u.Path)
	host := strings.ToLower(u.Hostname())
	return strings.Contains(path, "/thumbs/") ||
		strings.Contains(path, "/thumb/") ||
		strings.HasPrefix(host, "t3.") ||
		strings.Contains(host, "thumb")
}

func firstVipergirlsImage(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var images []string
	if err := json.Unmarshal([]byte(raw), &images); err != nil {
		return ""
	}
	var firstValid string
	for _, candidate := range images {
		candidate = strings.TrimSpace(candidate)
		if !isAbsoluteHTTPURL(candidate) {
			continue
		}
		if firstValid == "" {
			firstValid = candidate
		}
		if looksLikeVipergirlsThumbnail(candidate) {
			return candidate
		}
	}
	return firstValid
}

// ParseVipergirlsSearch extracts photo-subforum result cards from a vBulletin search
// results page. It is kept separate from the network fetch so it can be tested with
// synthetic HTML.
func ParseVipergirlsSearch(reader io.Reader, base *url.URL) ([]VipergirlsSearchResult, error) {
	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return nil, fmt.Errorf("parse vipergirls search page: %w", err)
	}

	rows := doc.Find("ol#searchbits li.threadbit")
	if rows.Length() == 0 {
		rows = doc.Find("li.threadbit")
	}

	results := make([]VipergirlsSearchResult, 0)
	seen := make(map[string]bool)
	rows.Each(func(_ int, row *goquery.Selection) {
		if len(results) >= vipergirlsMaxResults {
			return
		}

		// Some threads have a prefix link (e.g. "MetArtX") that also uses class
		// "title" and appears before the actual thread link. Select by the thread
		// href so those prefix links are never mistaken for the result.
		titleLink := row.Find(`h3.searchtitle a.title[href*="threads/"]`).First()
		if titleLink.Length() == 0 {
			return
		}
		title := strings.TrimSpace(titleLink.Text())
		href, ok := titleLink.Attr("href")
		if title == "" || !ok || strings.TrimSpace(href) == "" {
			return
		}

		subForum := strings.TrimSpace(row.Find("div.threadpostedin a").First().Text())
		if !isVipergirlsPhotoForum(subForum) {
			return
		}

		threadURL, err := canonicalVipergirlsURL(base, href)
		if err != nil || !IsVipergirlsThreadURL(threadURL) || seen[threadURL] {
			return
		}
		seen[threadURL] = true

		results = append(results, ForumSearchResult{
			Site:         ForumSiteVipergirls,
			ThreadID:     vipergirlsThreadID(threadURL),
			Title:        title,
			SubForum:     subForum,
			ThreadURL:    threadURL,
			ThumbnailURL: firstVipergirlsImage(titleLink.AttrOr("data-images", "")),
		})
	})

	return results, nil
}

// SearchVipergirls fetches the first page of a vipergirls.to search and returns only
// results from photo subforums.
func SearchVipergirls(query string) (*VipergirlsSearchResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), vipergirlsSearchTimeout)
	defer cancel()
	return SearchVipergirlsWithContext(ctx, query)
}

// SearchVipergirlsWithContext is the context-aware form used by the combined search.
func SearchVipergirlsWithContext(ctx context.Context, query string) (*VipergirlsSearchResponse, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("search query is required")
	}
	if len(q) > vipergirlsMaxQueryLen {
		return nil, fmt.Errorf("search query must be %d characters or fewer", vipergirlsMaxQueryLen)
	}

	searchURL := VipergirlsSearchURL(q)
	base, err := url.Parse(vipergirlsBaseURL + "/")
	if err != nil {
		return nil, fmt.Errorf("parse vipergirls base URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build vipergirls search request: %w", err)
	}
	req.Header.Set("User-Agent", vipergirlsUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := GetHTTPClient(searchURL).Do(req)
	if err != nil {
		return nil, fmt.Errorf("vipergirls search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vipergirls search returned status %d", resp.StatusCode)
	}

	results, err := ParseVipergirlsSearch(io.LimitReader(resp.Body, vipergirlsMaxBodySize), base)
	if err != nil {
		return nil, err
	}

	return &VipergirlsSearchResponse{Query: q, SearchURL: searchURL, Data: results}, nil
}

type vipergirlsSearchOutcome struct {
	response *VipergirlsSearchResponse
	err      error
}

type kittyKatsSearchOutcome struct {
	results []ForumSearchResult
	err     error
}

// collectForumSearch runs both site searches under one context. A failure from one
// site is reported in Sources while results from the other site are still returned.
func collectForumSearch(
	ctx context.Context,
	query string,
	searchVipergirls func(context.Context, string) (*VipergirlsSearchResponse, error),
	searchKittyKats func(context.Context, string) ([]ForumSearchResult, error),
) (*VipergirlsSearchResponse, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("search query is required")
	}
	if len(q) > vipergirlsMaxQueryLen {
		return nil, fmt.Errorf("search query must be %d characters or fewer", vipergirlsMaxQueryLen)
	}

	vgCh := make(chan vipergirlsSearchOutcome, 1)
	kkCh := make(chan kittyKatsSearchOutcome, 1)

	go func() {
		resp, err := searchVipergirls(ctx, q)
		vgCh <- vipergirlsSearchOutcome{response: resp, err: err}
	}()
	go func() {
		results, err := searchKittyKats(ctx, q)
		kkCh <- kittyKatsSearchOutcome{results: results, err: err}
	}()

	vg := <-vgCh
	kk := <-kkCh

	sources := []ForumSearchSource{
		{
			Site:      ForumSiteVipergirls,
			Label:     "vipergirls.to",
			SearchURL: VipergirlsSearchURL(q),
		},
		{
			Site:      ForumSiteKittyKats,
			Label:     "kitty-kats.net",
			SearchURL: KittyKatsSearchURL(q),
		},
	}
	if vg.err != nil {
		sources[0].Error = vg.err.Error()
	}
	if kk.err != nil {
		sources[1].Error = kk.err.Error()
	}

	data := make([]ForumSearchResult, 0)
	if vg.response != nil {
		data = append(data, vg.response.Data...)
	}
	data = append(data, kk.results...)

	searchURL := VipergirlsSearchURL(q)
	if vg.response != nil && vg.response.SearchURL != "" {
		searchURL = vg.response.SearchURL
	}
	response := &VipergirlsSearchResponse{
		Query:     q,
		SearchURL: searchURL,
		Sources:   sources,
		Data:      data,
	}
	if vg.err != nil && kk.err != nil {
		return response, fmt.Errorf("vipergirls search: %v; kitty-kats search: %v", vg.err, kk.err)
	}
	return response, nil
}

// SearchForumThreadsWithContext searches every supported forum for missing-gallery
// thread candidates. One unavailable site does not hide results from the other.
func SearchForumThreadsWithContext(ctx context.Context, query string) (*VipergirlsSearchResponse, error) {
	return collectForumSearch(ctx, query, SearchVipergirlsWithContext, SearchKittyKatsWithContext)
}

// ScrapeVipergirlsAsMissingGallery associates a forum thread with a person's missing
// gallery and queues the thread for crawling. It is safe to call twice for the same
// thread/person pair.
func ScrapeVipergirlsAsMissingGallery(req ScrapeVipergirlsRequest) (*ScrapeVipergirlsResponse, error) {
	if !IsForumThreadURL(req.ThreadURL) {
		return nil, fmt.Errorf("unsupported forum thread URL")
	}
	if req.ThumbnailURL != "" && !isAbsoluteHTTPURL(req.ThumbnailURL) {
		return nil, fmt.Errorf("invalid thumbnail URL")
	}

	var person models.Person
	if err := database.DB.First(&person, req.PersonID).Error; err != nil {
		return nil, fmt.Errorf("find person: %w", err)
	}

	name := strings.TrimSpace(req.GalleryName)
	if name == "" {
		name = strings.TrimSpace(req.Title)
	}
	if name == "" {
		return nil, fmt.Errorf("gallery name is required")
	}
	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		return nil, fmt.Errorf("provider is required")
	}

	// Keep the provider gallery URL on the gallery so provider scans recognise it as
	// resolved; fall back to the thread URL when only the forum result is known.
	sourceURL := strings.TrimSpace(req.MissingURL)
	if sourceURL == "" {
		sourceURL = req.ThreadURL
	}

	var gallery models.Gallery
	lookup := database.DB.Where("source_url = ?", req.ThreadURL)
	if strings.TrimSpace(req.MissingURL) != "" {
		lookup = database.DB.Where("source_url = ? OR source_url = ?", req.MissingURL, req.ThreadURL)
	}
	if err := lookup.First(&gallery).Error; err != nil {
		gallery = models.Gallery{Name: name, Provider: provider, SourceURL: sourceURL}
		if err := database.DB.Create(&gallery).Error; err != nil {
			return nil, fmt.Errorf("create gallery: %w", err)
		}
	} else {
		needsUpdate := false
		if gallery.Name == "" {
			gallery.Name = name
			needsUpdate = true
		}
		if gallery.Provider == "" {
			gallery.Provider = provider
			needsUpdate = true
		}
		if needsUpdate {
			database.DB.Save(&gallery)
		}
	}

	if req.ThumbnailURL != "" && gallery.ProviderThumbnail == "" && gallery.ProviderThumbnailURL == "" {
		// Record the source URL immediately and cache the bytes in the background.
		// Thumbnail hosts can be slow or retry for up to two minutes; that must not
		// keep the scrape request (and the UI's "Scraping..." state) open.
		gallery.ProviderThumbnailURL = req.ThumbnailURL
		database.DB.Save(&gallery)
		go cacheForumThumbnail(gallery.ID, req.ThumbnailURL)
	}

	var linkedCount int64
	database.DB.Table("person_galleries").
		Where("person_id = ? AND gallery_id = ?", person.ID, gallery.ID).
		Count(&linkedCount)
	if linkedCount == 0 {
		if err := database.DB.Model(&person).Association("Galleries").Append(&gallery); err != nil {
			return nil, fmt.Errorf("link gallery to person: %w", err)
		}
	}

	var source models.Source
	sourceExists := database.DB.Where("location = ?", req.ThreadURL).First(&source).Error == nil
	if !sourceExists {
		source = models.Source{
			Name:     name,
			Type:     "url",
			Location: req.ThreadURL,
			Status:   "idle",
		}
		if err := database.DB.Create(&source).Error; err != nil {
			return nil, fmt.Errorf("create source: %w", err)
		}
	}
	if gallery.SourceID == nil || *gallery.SourceID != source.ID {
		gallery.SourceID = &source.ID
		database.DB.Save(&gallery)
	}

	var queueErr error
	if !sourceExists || (source.Status != "queued" && source.Status != "crawling") {
		queueErr = QueueCrawlerSource(source.ID)
		if queueErr != nil {
			logger.Warnf("Failed to queue forum source %d for crawling: %v", source.ID, queueErr)
		}
	}

	if strings.TrimSpace(req.MissingURL) != "" {
		var existing models.ScanResultExclusion
		err := database.DB.Where("person_id = ? AND provider = ? AND source_url = ?",
			person.ID, provider, req.MissingURL).First(&existing).Error
		if err != nil {
			exclusion := models.ScanResultExclusion{
				PersonID:  person.ID,
				Provider:  provider,
				SourceURL: req.MissingURL,
				Title:     name,
				Reason:    "forum-scrape",
			}
			if err := database.DB.Create(&exclusion).Error; err != nil {
				logger.Warnf("Failed to exclude resolved missing gallery for person %d: %v", person.ID, err)
			}
		}
	}

	go refreshForumPersonScans(person.ID, provider)

	queued := queueErr == nil
	message := "Gallery linked and crawl queued"
	if sourceExists && (source.Status == "queued" || source.Status == "crawling") {
		message = "Gallery linked; crawl already running or queued"
	} else if queueErr != nil {
		message = "Gallery linked; source will resume crawling on startup"
	}
	return &ScrapeVipergirlsResponse{
		GalleryID: gallery.ID,
		SourceID:  source.ID,
		Queued:    queued,
		Message:   message,
	}, nil
}

// cacheForumThumbnail stores a thumbnail after the scrape request has already
// returned. It re-reads the gallery so it never overwrites a thumbnail that was
// cached or changed in the meantime.
func cacheForumThumbnail(galleryID uint, thumbnailURL string) {
	localPath, err := DownloadProviderThumbnail(thumbnailURL)
	if err != nil {
		logger.Warnf("Failed to download forum thumbnail for gallery %d: %v", galleryID, err)
		return
	}

	var gallery models.Gallery
	if err := database.DB.First(&gallery, galleryID).Error; err != nil {
		logger.Warnf("Failed to find gallery %d for forum thumbnail: %v", galleryID, err)
		return
	}
	if gallery.ProviderThumbnail != "" || gallery.ProviderThumbnailURL != thumbnailURL {
		return
	}
	gallery.ProviderThumbnail = localPath
	if err := database.DB.Save(&gallery).Error; err != nil {
		logger.Warnf("Failed to save forum thumbnail for gallery %d: %v", galleryID, err)
	}
}

func refreshForumPersonScans(personID uint, provider string) {
	var alias models.PersonProviderAlias
	if err := database.DB.Where("person_id = ? AND provider = ?", personID, provider).First(&alias).Error; err != nil {
		return
	}
	result, err := ScanSourceForPerson(personID, provider, alias.Alias)
	if err != nil {
		logger.Warnf("Failed to refresh scans for person %d after forum scrape: %v", personID, err)
		return
	}
	var latest models.PersonScanQueue
	if err := database.DB.Where("person_id = ? AND provider = ?", personID, provider).
		Order("created_at DESC").First(&latest).Error; err != nil {
		return
	}
	resultsJSON, err := json.Marshal(map[string]interface{}{
		"found_count":       result.FoundCount,
		"existing_count":    result.ExistingCount,
		"unsure_count":      result.UnsureCount,
		"missing_count":     result.MissingCount,
		"missing_galleries": result.MissingGalleries,
		"unsure_galleries":  result.UnsureGalleries,
	})
	if err != nil {
		return
	}
	latest.Results = string(resultsJSON)
	database.DB.Save(&latest)
}
