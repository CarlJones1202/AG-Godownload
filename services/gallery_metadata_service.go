package services

import (
	"encoding/json"
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
)

// GallerySearchResult represents a search result candidate
type GallerySearchResult struct {
	Provider    string `json:"provider"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Thumbnail   string `json:"thumbnail"`
	ReleaseDate string `json:"release_date"`
	SourceID    string `json:"source_id"` // UUID for MetArt or similar ID
	ID          uint   `json:"id"`        // Database gallery ID if matched
}

// GalleryMetadata represents scraped metadata from a gallery
type GalleryMetadata struct {
	Provider    string    `json:"provider"`
	Description string    `json:"description"`
	Rating      float64   `json:"rating"`
	ReleaseDate time.Time `json:"release_date"`
	SourceURL   string    `json:"source_url"`
	ThumbnailURL string   `json:"thumbnail_url"`
}

// SearchGalleryMatches searches for matching galleries across providers
func SearchGalleryMatches(galleryName string, people []string) ([]GallerySearchResult, error) {
	var results []GallerySearchResult

	// Build search query - User requested ONLY gallery name for all sources
	// Adding people/models often breaks the specific search algorithms of these sites
	searchQuery := galleryName
	logger.Infof("Searching for gallery matches with query: %s", searchQuery)

	// Search MetArt
	metartResults, err := searchMetArt(searchQuery)
	if err != nil {
		logger.Warnf("MetArt search failed: %v", err)
	} else {
		results = append(results, metartResults...)
	}

	// Search MetartX
	metartxResults, err := searchMetartX(searchQuery)
	if err != nil {
		logger.Warnf("MetartX search failed: %v", err)
	} else {
		results = append(results, metartxResults...)
	}

	// Search Playboy
	playboyResults, err := searchPlayboy(searchQuery)
	if err != nil {
		logger.Warnf("Playboy search failed: %v", err)
	} else {
		results = append(results, playboyResults...)
	}

	// Search PlayboyPlus
	playboyPlusResults, err := searchPlayboyPlus(searchQuery)
	if err != nil {
		logger.Warnf("PlayboyPlus search failed: %v", err)
	} else {
		results = append(results, playboyPlusResults...)
	}

	// Search Vixen
	vixenResults, err := searchVixen(searchQuery)
	if err != nil {
		logger.Warnf("Vixen search failed: %v", err)
	} else {
		results = append(results, vixenResults...)
	}

	// Search SexArt
	sexartResults, err := searchSexArt(searchQuery)
	if err != nil {
		logger.Warnf("SexArt search failed: %v", err)
	} else {
		results = append(results, sexartResults...)
	}

	// Search LifeErotic
	lifeeroticResults, err := searchLifeErotic(searchQuery)
	if err != nil {
		logger.Warnf("LifeErotic search failed: %v", err)
	} else {
		results = append(results, lifeeroticResults...)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no matching galleries found")
	}

	logger.Infof("Found %d matching galleries", len(results))
	return results, nil
}

// ScrapeGalleryMetadata scrapes full metadata from a confirmed gallery URL
func ScrapeGalleryMetadata(sourceURL string, provider string, sourceID string) (*GalleryMetadata, error) {
	logger.Infof("Scraping metadata from %s (%s) ID: %s", sourceURL, provider, sourceID)

	switch strings.ToLower(provider) {
	case "metart":
		return scrapeMetArtGallery(sourceURL, sourceID)
	case "metartx":
		return scrapeMetartXGallery(sourceURL, sourceID)
	case "playboy":
		return scrapePlayboyGallery(sourceURL)
	case "playboyplus":
		return scrapePlayboyPlusGallery(sourceURL)
	case "vixen":
		return scrapeVixenGallery(sourceURL)
	case "sexart":
		return scrapeSexArtGallery(sourceURL, sourceID)
	case "lifeerotic":
		return scrapeLifeEroticGallery(sourceURL, sourceID)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

// buildSearchQuery constructs a search query from gallery name and people
func buildSearchQuery(galleryName string, people []string) string {
	parts := []string{galleryName}
	for _, person := range people {
		if person != "" {
			parts = append(parts, person)
		}
	}
	return strings.Join(parts, " ")
}

// searchMetArt searches MetArt for matching galleries using their internal API
func searchMetArt(query string) ([]GallerySearchResult, error) {
	// Use the internal API endpoint discovered via reverse engineering
	// The site is an SPA, so HTML scraping returns empty shells.
	// API: https://www.metart.com/api/search-results?searchPhrase={QUERY}&page=1&pageSize=30&sortBy=latest-gallery

	apiURL := fmt.Sprintf("https://www.metart.com/api/search-results?searchPhrase=%s&page=1&pageSize=30&sortBy=latest-gallery", url.QueryEscape(query))
	logger.Infof("[MetArt] Searching API: %s", apiURL)

	client := GetHTTPClient(apiURL)
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search MetArt API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("MetArt API returned status %d", resp.StatusCode)
	}

	// Parse JSON Response based on the schema reverse-engineered
	var apiResp struct {
		Items []struct {
			Item struct {
				Name        string `json:"name"`
				Path        string `json:"path"`
				PublishedAt string `json:"publishedAt"`
				Thumbnail   string `json:"thumbnailCoverPath"`
				Models      []struct {
					Name string `json:"name"`
				} `json:"models"`
			} `json:"item"`
		} `json:"items"`
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		logger.Errorf("[MetArt] Failed to parse JSON: %v", err)
		// Save for debugging
		os.WriteFile("debug_metart_api_error.json", bodyBytes, 0644)
		return nil, fmt.Errorf("failed to parse API JSON")
	}

	var results []GallerySearchResult
	for _, entry := range apiResp.Items {
		item := entry.Item

		// Construct full URLs
		galleryURL := "https://www.metart.com" + item.Path

		// For thumbnails, using the relative path on main domain
		thumbURL := "https://www.metart.com" + item.Thumbnail

		// Parse date (2018-10-20T07:00:00.000Z)
		dateStr := item.PublishedAt
		if len(dateStr) > 10 {
			dateStr = dateStr[:10]
		}

		results = append(results, GallerySearchResult{
			Provider:    "MetArt",
			Title:       item.Name,
			URL:         galleryURL,
			Thumbnail:   thumbURL,
			ReleaseDate: dateStr,
		})
	}

	logger.Infof("[MetArt] Found %d results via API", len(results))
	return results, nil
}

// searchMetartX searches MetartX for matching galleries using their internal API
func searchMetartX(query string) ([]GallerySearchResult, error) {
	apiURL := fmt.Sprintf("https://www.metartx.com/api/search-results?searchPhrase=%s&page=1&pageSize=30&sortBy=latest-gallery", url.QueryEscape(query))
	logger.Infof("[MetartX] Searching API: %s", apiURL)

	client := GetHTTPClient(apiURL)
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search MetartX API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("MetartX API returned status %d", resp.StatusCode)
	}

	var apiResp struct {
		Items []struct {
			Item struct {
				Name        string `json:"name"`
				Path        string `json:"path"`
				PublishedAt string `json:"publishedAt"`
				Thumbnail   string `json:"thumbnailCoverPath"`
				Models      []struct {
					Name string `json:"name"`
				} `json:"models"`
			} `json:"item"`
		} `json:"items"`
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		logger.Errorf("[MetartX] Failed to parse JSON: %v", err)
		return nil, fmt.Errorf("failed to parse API JSON")
	}

	var results []GallerySearchResult
	for _, entry := range apiResp.Items {
		item := entry.Item

		galleryURL := "https://www.metartx.com" + item.Path
		thumbURL := "https://www.metartx.com" + item.Thumbnail

		dateStr := item.PublishedAt
		if len(dateStr) > 10 {
			dateStr = dateStr[:10]
		}

		results = append(results, GallerySearchResult{
			Provider:    "MetartX",
			Title:       item.Name,
			URL:         galleryURL,
			Thumbnail:   thumbURL,
			ReleaseDate: dateStr,
		})
	}

	logger.Infof("[MetartX] Found %d results via API", len(results))
	return results, nil
}

// searchSexArt searches SexArt for matching galleries using their internal API
func searchSexArt(query string) ([]GallerySearchResult, error) {
	apiURL := fmt.Sprintf("https://www.sexart.com/api/search-results?searchPhrase=%s&page=1&pageSize=30&sortBy=latest-gallery", url.QueryEscape(query))
	logger.Infof("[SexArt] Searching API: %s", apiURL)

	client := GetHTTPClient(apiURL)
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search SexArt API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("SexArt API returned status %d", resp.StatusCode)
	}

	var apiResp struct {
		Items []struct {
			Item struct {
				Name        string `json:"name"`
				Path        string `json:"path"`
				PublishedAt string `json:"publishedAt"`
				Thumbnail   string `json:"thumbnailCoverPath"`
				Models      []struct {
					Name string `json:"name"`
				} `json:"models"`
			} `json:"item"`
		} `json:"items"`
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		logger.Errorf("[SexArt] Failed to parse JSON: %v", err)
		return nil, fmt.Errorf("failed to parse API JSON")
	}

	var results []GallerySearchResult
	for _, entry := range apiResp.Items {
		item := entry.Item

		galleryURL := "https://www.sexart.com" + item.Path
		thumbURL := "https://www.sexart.com" + item.Thumbnail

		dateStr := item.PublishedAt
		if len(dateStr) > 10 {
			dateStr = dateStr[:10]
		}

		results = append(results, GallerySearchResult{
			Provider:    "SexArt",
			Title:       item.Name,
			URL:         galleryURL,
			Thumbnail:   thumbURL,
			ReleaseDate: dateStr,
		})
	}

	logger.Infof("[SexArt] Found %d results via API", len(results))
	return results, nil
}

// searchLifeErotic searches The Life Erotic for matching galleries using their internal API
func searchLifeErotic(query string) ([]GallerySearchResult, error) {
	apiURL := fmt.Sprintf("https://www.thelifeerotic.com/api/search-results?searchPhrase=%s&page=1&pageSize=30&sortBy=latest-gallery", url.QueryEscape(query))
	logger.Infof("[LifeErotic] Searching API: %s", apiURL)

	client := GetHTTPClient(apiURL)
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search LifeErotic API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("LifeErotic API returned status %d", resp.StatusCode)
	}

	var apiResp struct {
		Items []struct {
			Item struct {
				Name        string `json:"name"`
				Path        string `json:"path"`
				PublishedAt string `json:"publishedAt"`
				Thumbnail   string `json:"thumbnailCoverPath"`
				Models      []struct {
					Name string `json:"name"`
				} `json:"models"`
			} `json:"item"`
		} `json:"items"`
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		logger.Errorf("[LifeErotic] Failed to parse JSON: %v", err)
		return nil, fmt.Errorf("failed to parse API JSON")
	}

	var results []GallerySearchResult
	for _, entry := range apiResp.Items {
		item := entry.Item

		galleryURL := "https://www.thelifeerotic.com" + item.Path
		thumbURL := "https://www.thelifeerotic.com" + item.Thumbnail

		dateStr := item.PublishedAt
		if len(dateStr) > 10 {
			dateStr = dateStr[:10]
		}

		results = append(results, GallerySearchResult{
			Provider:    "LifeErotic",
			Title:       item.Name,
			URL:         galleryURL,
			Thumbnail:   thumbURL,
			ReleaseDate: dateStr,
		})
	}

	logger.Infof("[LifeErotic] Found %d results via API", len(results))
	return results, nil
}

// searchPlayboy searches Playboy for matching galleries
func searchPlayboy(query string) ([]GallerySearchResult, error) {
	searchURL := fmt.Sprintf("https://www.playboy.com/search?q=%s", strings.ReplaceAll(query, " ", "+"))

	client := GetHTTPClient(searchURL)
	resp, err := client.Get(searchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search Playboy: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Playboy search returned status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Playboy search results: %w", err)
	}

	var results []GallerySearchResult

	// Parse search results - placeholder selectors
	doc.Find(".gallery-item, .search-result, article").Each(func(i int, s *goquery.Selection) {
		if i >= 10 {
			return
		}

		title := strings.TrimSpace(s.Find("h2, h3, .title").First().Text())
		url, _ := s.Find("a").First().Attr("href")
		thumbnail, _ := s.Find("img").First().Attr("src")
		releaseDate := strings.TrimSpace(s.Find(".date, time").First().Text())

		if url != "" && !strings.HasPrefix(url, "http") {
			url = "https://www.playboy.com" + url
		}

		if thumbnail != "" && !strings.HasPrefix(thumbnail, "http") {
			thumbnail = "https://www.playboy.com" + thumbnail
		}

		if title != "" && url != "" {
			results = append(results, GallerySearchResult{
				Provider:    "Playboy",
				Title:       title,
				URL:         url,
				Thumbnail:   thumbnail,
				ReleaseDate: releaseDate,
			})
		}
	})

	logger.Debugf("Found %d Playboy results", len(results))
	return results, nil
}

// scrapeMetArtGallery scrapes full metadata from a MetArt gallery page/API
func scrapeMetArtGallery(urlStr, uuid string) (*GalleryMetadata, error) {
	// The user identified that the correct API endpoint uses name and date parameter
	// Format: https://www.metart.com/api/gallery?name=PRESENTING_LIBBY&date=20181020
	// We can extract these from the source URL:
	// https://www.metart.com/model/libby/gallery/20181020/PRESENTING_LIBBY

	// Parse the URL to extract date and name
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid source URL: %w", err)
	}

	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(pathParts) < 2 {
		return nil, fmt.Errorf("invalid MetArt URL format, cannot extract date/name")
	}

	// Assuming standard format .../gallery/{date}/{name}
	// We look for 'gallery' segment and take the next two
	var galleryDate, galleryName string
	for i, part := range pathParts {
		if part == "gallery" && i+2 < len(pathParts) {
			galleryDate = pathParts[i+1]
			galleryName = pathParts[i+2]
			break
		}
	}

	// Fallback: take last two if "gallery" keyword not found (e.g. different structure)
	if galleryDate == "" && len(pathParts) >= 2 {
		galleryDate = pathParts[len(pathParts)-2]
		galleryName = pathParts[len(pathParts)-1]
	}

	apiURL := fmt.Sprintf("https://www.metart.com/api/gallery?name=%s&date=%s", galleryName, galleryDate)
	logger.Infof("[MetArt] Fetching detail API: %s", apiURL)

	client := GetHTTPClient(apiURL)
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch MetArt gallery API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("MetArt gallery API returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read MetArt response: %w", err)
	}

	// Parse JSON
	var galleryDetail struct {
		Name          string  `json:"name"`
		Description   string  `json:"description"`
		RatingAverage float64 `json:"ratingAverage"`
		PublishedAt   string  `json:"publishedAt"`
		CoverImageURL string  `json:"coverImageUrl"`
	}

	if err := json.Unmarshal(bodyBytes, &galleryDetail); err != nil {
		logger.Errorf("[MetArt] Failed to parse detail JSON: %v", err)
		return nil, fmt.Errorf("failed to parse detail JSON")
	}

	metadata := &GalleryMetadata{
		Provider:    "MetArt",
		SourceURL:   urlStr,
		Description: galleryDetail.Description,
		Rating:      galleryDetail.RatingAverage,
	}

	// Try to get thumbnail from API response or construct from path
	if galleryDetail.CoverImageURL != "" {
		metadata.ThumbnailURL = "https://www.metart.com" + galleryDetail.CoverImageURL
	} else {
		// Construct thumbnail URL from gallery name
		metadata.ThumbnailURL = fmt.Sprintf("https://www.metart.com/photo/%s/0/0.jpg", strings.ToLower(galleryName))
	}

	// Parse date
	if galleryDetail.PublishedAt != "" {
		if parsed, err := time.Parse("2006-01-02T15:04:05.000Z", galleryDetail.PublishedAt); err == nil {
			metadata.ReleaseDate = parsed
		}
	}

	logger.Infof("Scraped MetArt gallery: %s (Rating: %.2f)", metadata.Description[:min(50, len(metadata.Description))], metadata.Rating)
	return metadata, nil
}

// scrapeMetartXGallery scrapes full metadata from a MetartX gallery page
func scrapeMetartXGallery(urlStr, uuid string) (*GalleryMetadata, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid source URL: %w", err)
	}

	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(pathParts) < 2 {
		return nil, fmt.Errorf("invalid MetartX URL format, cannot extract date/name")
	}

	var galleryDate, galleryName string
	for i, part := range pathParts {
		if part == "gallery" && i+2 < len(pathParts) {
			galleryDate = pathParts[i+1]
			galleryName = pathParts[i+2]
			break
		}
	}

	if galleryDate == "" && len(pathParts) >= 2 {
		galleryDate = pathParts[len(pathParts)-2]
		galleryName = pathParts[len(pathParts)-1]
	}

	apiURL := fmt.Sprintf("https://www.metartx.com/api/gallery?name=%s&date=%s", galleryName, galleryDate)
	logger.Infof("[MetartX] Fetching detail API: %s", apiURL)

	client := GetHTTPClient(apiURL)
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch MetartX gallery API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("MetartX gallery API returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read MetartX response: %w", err)
	}

	var galleryDetail struct {
		Name          string  `json:"name"`
		Description   string  `json:"description"`
		RatingAverage float64 `json:"ratingAverage"`
		PublishedAt   string  `json:"publishedAt"`
	}

	if err := json.Unmarshal(bodyBytes, &galleryDetail); err != nil {
		logger.Errorf("[MetartX] Failed to parse detail JSON: %v", err)
		return nil, fmt.Errorf("failed to parse detail JSON")
	}

	metadata := &GalleryMetadata{
		Provider:    "MetartX",
		SourceURL:   urlStr,
		Description: galleryDetail.Description,
		Rating:      galleryDetail.RatingAverage,
	}

	if galleryDetail.PublishedAt != "" {
		if parsed, err := time.Parse("2006-01-02T15:04:05.000Z", galleryDetail.PublishedAt); err == nil {
			metadata.ReleaseDate = parsed
		}
	}

	logger.Infof("Scraped MetartX gallery: %s (Rating: %.2f)", metadata.Description[:min(50, len(metadata.Description))], metadata.Rating)
	return metadata, nil
}

// scrapeSexArtGallery scrapes full metadata from a SexArt gallery page
func scrapeSexArtGallery(urlStr, uuid string) (*GalleryMetadata, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid source URL: %w", err)
	}

	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(pathParts) < 2 {
		return nil, fmt.Errorf("invalid SexArt URL format, cannot extract date/name")
	}

	var galleryDate, galleryName string
	for i, part := range pathParts {
		if part == "gallery" && i+2 < len(pathParts) {
			galleryDate = pathParts[i+1]
			galleryName = pathParts[i+2]
			break
		}
	}

	if galleryDate == "" && len(pathParts) >= 2 {
		galleryDate = pathParts[len(pathParts)-2]
		galleryName = pathParts[len(pathParts)-1]
	}

	apiURL := fmt.Sprintf("https://www.sexart.com/api/gallery?name=%s&date=%s", galleryName, galleryDate)
	logger.Infof("[SexArt] Fetching detail API: %s", apiURL)

	client := GetHTTPClient(apiURL)
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch SexArt gallery API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("SexArt gallery API returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read SexArt response: %w", err)
	}

	var galleryDetail struct {
		Name          string  `json:"name"`
		Description   string  `json:"description"`
		RatingAverage float64 `json:"ratingAverage"`
		PublishedAt   string  `json:"publishedAt"`
	}

	if err := json.Unmarshal(bodyBytes, &galleryDetail); err != nil {
		logger.Errorf("[SexArt] Failed to parse detail JSON: %v", err)
		return nil, fmt.Errorf("failed to parse detail JSON")
	}

	metadata := &GalleryMetadata{
		Provider:    "SexArt",
		SourceURL:   urlStr,
		Description: galleryDetail.Description,
		Rating:      galleryDetail.RatingAverage,
	}

	if galleryDetail.PublishedAt != "" {
		if parsed, err := time.Parse("2006-01-02T15:04:05.000Z", galleryDetail.PublishedAt); err == nil {
			metadata.ReleaseDate = parsed
		}
	}

	logger.Infof("Scraped SexArt gallery: %s (Rating: %.2f)", metadata.Description[:min(50, len(metadata.Description))], metadata.Rating)
	return metadata, nil
}

// scrapeLifeEroticGallery scrapes full metadata from a LifeErotic gallery page
func scrapeLifeEroticGallery(urlStr, uuid string) (*GalleryMetadata, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid source URL: %w", err)
	}

	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(pathParts) < 2 {
		return nil, fmt.Errorf("invalid LifeErotic URL format, cannot extract date/name")
	}

	var galleryDate, galleryName string
	for i, part := range pathParts {
		if part == "gallery" && i+2 < len(pathParts) {
			galleryDate = pathParts[i+1]
			galleryName = pathParts[i+2]
			break
		}
	}

	if galleryDate == "" && len(pathParts) >= 2 {
		galleryDate = pathParts[len(pathParts)-2]
		galleryName = pathParts[len(pathParts)-1]
	}

	apiURL := fmt.Sprintf("https://www.thelifeerotic.com/api/gallery?name=%s&date=%s", galleryName, galleryDate)
	logger.Infof("[LifeErotic] Fetching detail API: %s", apiURL)

	client := GetHTTPClient(apiURL)
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch LifeErotic gallery API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("LifeErotic gallery API returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read LifeErotic response: %w", err)
	}

	var galleryDetail struct {
		Name          string  `json:"name"`
		Description   string  `json:"description"`
		RatingAverage float64 `json:"ratingAverage"`
		PublishedAt   string  `json:"publishedAt"`
	}

	if err := json.Unmarshal(bodyBytes, &galleryDetail); err != nil {
		logger.Errorf("[LifeErotic] Failed to parse detail JSON: %v", err)
		return nil, fmt.Errorf("failed to parse detail JSON")
	}

	metadata := &GalleryMetadata{
		Provider:    "LifeErotic",
		SourceURL:   urlStr,
		Description: galleryDetail.Description,
		Rating:      galleryDetail.RatingAverage,
	}

	if galleryDetail.PublishedAt != "" {
		if parsed, err := time.Parse("2006-01-02T15:04:05.000Z", galleryDetail.PublishedAt); err == nil {
			metadata.ReleaseDate = parsed
		}
	}

	logger.Infof("Scraped LifeErotic gallery: %s (Rating: %.2f)", metadata.Description[:min(50, len(metadata.Description))], metadata.Rating)
	return metadata, nil
}

// scrapePlayboyGallery scrapes full metadata from a Playboy gallery page
func scrapePlayboyGallery(url string) (*GalleryMetadata, error) {
	client := GetHTTPClient(url)
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Playboy gallery: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Playboy gallery returned status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Playboy gallery: %w", err)
	}

	metadata := &GalleryMetadata{
		Provider:  "Playboy",
		SourceURL: url,
	}

	// Extract description
	description := strings.TrimSpace(doc.Find(".description, .synopsis, p.description").First().Text())
	metadata.Description = description

	// Extract rating
	ratingText := strings.TrimSpace(doc.Find(".rating, .score").First().Text())
	if ratingText != "" {
		fmt.Sscanf(ratingText, "%f", &metadata.Rating)
	}

	// Extract release date
	dateText := strings.TrimSpace(doc.Find(".date, time, .publish-date").First().Text())
	if dateText != "" {
		for _, layout := range []string{"2006-01-02", "January 2, 2006", "Jan 2, 2006", "02/01/2006"} {
			if parsed, err := time.Parse(layout, dateText); err == nil {
				metadata.ReleaseDate = parsed
				break
			}
		}
	}

	logger.Infof("Scraped Playboy gallery: %s", metadata.Description[:min(50, len(metadata.Description))])
	return metadata, nil
}

// searchVixen searches Vixen Media Group (VMG) for matching videos/galleries
func searchVixen(query string) ([]GallerySearchResult, error) {
	// GraphQL query for searching VMG
	gqlQuery := map[string]interface{}{
		"query": `
			query search($term: String) {
				search(term: $term) {
					videos {
						id
						title
						slug
						releaseDate
						images {
							poster {
								url
							}
						}
					}
				}
			}
		`,
		"variables": map[string]string{
			"term": query,
		},
	}

	bodyBytes, err := json.Marshal(gqlQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GraphQL query: %w", err)
	}

	apiURL := "https://www.vixen.com/graphql"
	client := GetHTTPClient(apiURL)
	resp, err := client.Post(apiURL, "application/json", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to search Vixen API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Vixen API returned status %d", resp.StatusCode)
	}

	var apiResp struct {
		Data struct {
			Search struct {
				Videos []struct {
					ID          string `json:"id"`
					Title       string `json:"title"`
					Slug        string `json:"slug"`
					ReleaseDate string `json:"releaseDate"`
					Images      struct {
						Poster []struct {
							URL string `json:"url"`
						} `json:"poster"`
					} `json:"images"`
				} `json:"videos"`
			} `json:"search"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode Vixen API response: %w", err)
	}

	var results []GallerySearchResult
	for _, video := range apiResp.Data.Search.Videos {
		thumbURL := ""
		if len(video.Images.Poster) > 0 {
			thumbURL = video.Images.Poster[0].URL
		}

		results = append(results, GallerySearchResult{
			Provider:    "Vixen",
			Title:       video.Title,
			URL:         "https://www.vixen.com/videos/" + video.Slug,
			Thumbnail:   thumbURL,
			ReleaseDate: video.ReleaseDate,
			SourceID:    video.ID,
		})
	}

	return results, nil
}

// scrapeVixenGallery scrapes full metadata from a Vixen video page using GraphQL
func scrapeVixenGallery(urlStr string) (*GalleryMetadata, error) {
	// Extract slug from URL
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid Vixen URL: %w", err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "videos" {
		return nil, fmt.Errorf("invalid Vixen video URL format")
	}
	slug := parts[len(parts)-1]

	gqlQuery := map[string]interface{}{
		"query": `
			query findOneVideo($id: Int, $slug: String) {
				findOneVideo(input: { id: $id, slug: $slug }) {
					id
					title
					description
					releaseDate
					models {
						name
					}
				}
			}
		`,
		"variables": map[string]string{
			"slug": slug,
		},
	}

	bodyBytes, err := json.Marshal(gqlQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GraphQL query: %w", err)
	}

	apiURL := "https://www.vixen.com/graphql"
	client := GetHTTPClient(apiURL)
	resp, err := client.Post(apiURL, "application/json", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to scrape Vixen API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Vixen API returned status %d", resp.StatusCode)
	}

	var apiResp struct {
		Data struct {
			FindOneVideo struct {
				ID          string `json:"id"`
				Title       string `json:"title"`
				Description string `json:"description"`
				ReleaseDate string `json:"releaseDate"`
				Models      []struct {
					Name string `json:"name"`
				} `json:"models"`
			} `json:"findOneVideo"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode Vixen API response: %w", err)
	}

	video := apiResp.Data.FindOneVideo
	metadata := &GalleryMetadata{
		Provider:    "Vixen",
		SourceURL:   urlStr,
		Description: video.Description,
	}

	if video.ReleaseDate != "" {
		// Try ISO 8601 first
		if parsed, err := time.Parse("2006-01-02T15:04:05.000Z", video.ReleaseDate); err == nil {
			metadata.ReleaseDate = parsed
		} else if parsed, err := time.Parse("2006-01-02", video.ReleaseDate[:10]); err == nil {
			metadata.ReleaseDate = parsed
		}
	}

	return metadata, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// searchPlayboyPlus searches PlayboyPlus using their Algolia API
func searchPlayboyPlus(query string) ([]GallerySearchResult, error) {
	// Algolia API info from their public site
	appID := "TSMKFA364Q"
	// Note: This key is short-lived and extracted from window.env on playboyplus.com
	apiKey := "Mzc0ZWExYWJiMGIzMWNmYTFhYWEyZjg1Y2VjY2NlOTFiMDA4OWZlMzU3NzBkZWYzZDYzMmE3MWNhZTY5MGI2NHZhbGlkVW50aWw9MTc3MjIyODc5NyZyZXN0cmljdEluZGljZXM9YWxsJTJBJmZpbHRlcnM9c2VnbWVudCUzQXBsYXlib3lwbHVz"
	indexName := "all_photosets"

	apiURL := fmt.Sprintf("https://%s-dsn.algolia.net/1/indexes/%s/query", appID, indexName)

	searchParams := map[string]interface{}{
		"params": fmt.Sprintf("query=%s&hitsPerPage=20&filters=segment:playboyplus", url.QueryEscape(query)),
	}

	bodyBytes, err := json.Marshal(searchParams)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Algolia query: %w", err)
	}

	client := GetHTTPClient(apiURL)
	req, _ := http.NewRequest("POST", apiURL, strings.NewReader(string(bodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Algolia-Application-Id", appID)
	req.Header.Set("X-Algolia-API-Key", apiKey)
	req.Header.Set("Referer", "https://www.playboyplus.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to search PlayboyPlus Algolia: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("PlayboyPlus Algolia returned status %d", resp.StatusCode)
	}

	var apiResp struct {
		Hits []struct {
			ObjectID    string `json:"objectID"`
			Title       string `json:"title"`
			URLTitle    string `json:"urlTitle"`
			ReleaseDate string `json:"release_date"`
			Thumbnails  struct {
				Standard string `json:"standard"`
			} `json:"thumbnails"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode PlayboyPlus Algolia response: %w", err)
	}

	var results []GallerySearchResult
	for _, hit := range apiResp.Hits {
		results = append(results, GallerySearchResult{
			Provider:    "PlayboyPlus",
			Title:       hit.Title,
			URL:         fmt.Sprintf("https://www.playboyplus.com/en/update/%s/%s", hit.URLTitle, hit.ObjectID),
			Thumbnail:   hit.Thumbnails.Standard,
			ReleaseDate: hit.ReleaseDate,
			SourceID:    hit.ObjectID,
		})
	}

	return results, nil
}

// searchPlayboyPlusByModel searches PlayboyPlus for galleries by model name
// First searches for the model, then gets their galleries
func searchPlayboyPlusByModel(modelName string) ([]GallerySearchResult, error) {
	appID := "TSMKFA364Q"
	apiKey := "Mzc0ZWExYWJiMGIzMWNmYTFhYWEyZjg1Y2VjY2NlOTFiMDA4OWZlMzU3NzBkZWYzZDYzMmE3MWNhZTY5MGI2NHZhbGlkVW50aWw9MTc3MjIyODc5NyZyZXN0cmljdEluZGljZXM9YWxsJTJBJmZpbHRlcnM9c2VnbWVudCUzQXBsYXlib3lwbHVz"

	// Step 1: Search for the model in all_actors index
	actorsURL := fmt.Sprintf("https://%s-dsn.algolia.net/1/indexes/all_actors/query", appID)

	actorsParams := map[string]interface{}{
		"params": fmt.Sprintf("query=%s&hitsPerPage=10", url.QueryEscape(modelName)),
	}

	actorsBody, err := json.Marshal(actorsParams)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal actors query: %w", err)
	}

	logger.Infof("[PlayboyPlus] Step 1 - Searching actors URL: %s", actorsURL)
	logger.Infof("[PlayboyPlus] Step 1 - Request body: %s", string(actorsBody))

	client := GetHTTPClient(actorsURL)
	req, _ := http.NewRequest("POST", actorsURL, strings.NewReader(string(actorsBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Algolia-Application-Id", appID)
	req.Header.Set("X-Algolia-API-Key", apiKey)
	req.Header.Set("Referer", "https://www.playboyplus.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to search PlayboyPlus actors: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	logger.Infof("[PlayboyPlus] Step 1 - Response status: %d", resp.StatusCode)
	logger.Infof("[PlayboyPlus] Step 1 - Response body: %s", string(bodyBytes))

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("PlayboyPlus actors search returned status %d", resp.StatusCode)
	}

	var actorsResp struct {
		Hits []struct {
			ObjectID string `json:"objectID"`
			Name     string `json:"name"`
			URLName string `json:"urlName"`
		} `json:"hits"`
	}

	if err := json.Unmarshal(bodyBytes, &actorsResp); err != nil {
		return nil, fmt.Errorf("failed to decode PlayboyPlus actors response: %w", err)
	}

	logger.Infof("[PlayboyPlus] Step 1 - Found %d actor hits", len(actorsResp.Hits))
	for i, actor := range actorsResp.Hits {
		logger.Infof("[PlayboyPlus] Step 1 - Actor %d: Name=%s, ID=%s, URLName=%s", i, actor.Name, actor.ObjectID, actor.URLName)
	}

	// Find exact match
	var actorID string
	for _, actor := range actorsResp.Hits {
		// Case-insensitive exact match
		if strings.EqualFold(actor.Name, modelName) {
			actorID = actor.ObjectID
			logger.Infof("[PlayboyPlus] Found exact model match: %s (ID: %s)", actor.Name, actorID)
			break
		}
	}

	if actorID == "" {
		logger.Infof("[PlayboyPlus] No exact model match found for: %s", modelName)
		return nil, nil
	}

	// Step 2: Search for galleries by this actor using the format the website uses
	galleriesURL := fmt.Sprintf("https://%s-dsn.algolia.net/1/indexes/all_photosets/query", appID)

	// Use the same format the website uses - facetFilters with nested arrays
	galleriesParams := map[string]interface{}{
		"params": fmt.Sprintf("hitsPerPage=50&facetFilters=[[\\\"actors.id:%s\\\"]]", actorID),
	}

	galleriesBody, err := json.Marshal(galleriesParams)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal galleries query: %w", err)
	}

	logger.Infof("[PlayboyPlus] Step 2 - Searching galleries URL: %s", galleriesURL)
	logger.Infof("[PlayboyPlus] Step 2 - Request body: %s", string(galleriesBody))

	req2, _ := http.NewRequest("POST", galleriesURL, strings.NewReader(string(galleriesBody)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Algolia-Application-Id", appID)
	req2.Header.Set("X-Algolia-API-Key", apiKey)
	req2.Header.Set("Referer", "https://www.playboyplus.com/")

	resp2, err := client.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("failed to search PlayboyPlus galleries: %w", err)
	}
	defer resp2.Body.Close()

	bodyBytes2, _ := io.ReadAll(resp2.Body)
	logger.Infof("[PlayboyPlus] Step 2 - Response status: %d", resp2.StatusCode)
	logger.Infof("[PlayboyPlus] Step 2 - Response body: %s", string(bodyBytes2))

	if resp2.StatusCode != 200 {
		return nil, fmt.Errorf("PlayboyPlus galleries search returned status %d", resp2.StatusCode)
	}

	var galleriesResp struct {
		Hits []struct {
			ObjectID    string `json:"objectID"`
			Title       string `json:"title"`
			URLTitle    string `json:"urlTitle"`
			ReleaseDate string `json:"release_date"`
			Thumbnails  struct {
				Standard string `json:"standard"`
			} `json:"thumbnails"`
		} `json:"hits"`
	}

	if err := json.Unmarshal(bodyBytes2, &galleriesResp); err != nil {
		return nil, fmt.Errorf("failed to decode PlayboyPlus galleries response: %w", err)
	}

	logger.Infof("[PlayboyPlus] Step 2 - Found %d gallery hits", len(galleriesResp.Hits))

	var results []GallerySearchResult
	for _, hit := range galleriesResp.Hits {
		logger.Infof("[PlayboyPlus] Step 2 - Gallery: Title=%s, ID=%s", hit.Title, hit.ObjectID)
		results = append(results, GallerySearchResult{
			Provider:    "PlayboyPlus",
			Title:       hit.Title,
			URL:         fmt.Sprintf("https://www.playboyplus.com/en/update/%s/%s", hit.URLTitle, hit.ObjectID),
			Thumbnail:   hit.Thumbnails.Standard,
			ReleaseDate: hit.ReleaseDate,
			SourceID:    hit.ObjectID,
		})
	}

	logger.Infof("[PlayboyPlus] Found %d galleries for model %s", len(results), modelName)
	return results, nil
}

type PersonScanResult struct {
	PersonID         uint                  `json:"person_id"`
	PersonName       string                `json:"person_name"`
	Provider         string                `json:"provider"`
	FoundCount       int                   `json:"found_count"`
	ExistingCount    int                   `json:"existing_count"`
	UnsureCount      int                   `json:"unsure_count"`
	MissingCount     int                   `json:"missing_count"`
	MissingGalleries []GallerySearchResult `json:"missing_galleries"`
	UnsureGalleries  []GallerySearchResult `json:"unsure_galleries"`
}

func ScanSourceForPerson(personID uint, provider string, alias string) (*PersonScanResult, error) {
	var person models.Person
	if err := database.DB.First(&person, personID).Error; err != nil {
		return nil, fmt.Errorf("person not found: %w", err)
	}

	var searchTerms []string
	if alias != "" {
		searchTerms = []string{alias}
	} else {
		searchTerms = []string{person.Name}
		if person.Aliases != "" {
			var aliases []string
			if err := json.Unmarshal([]byte(person.Aliases), &aliases); err == nil {
				searchTerms = append(searchTerms, aliases...)
			}
		}
	}

	var allResults []GallerySearchResult

	switch strings.ToLower(provider) {
	case "metart":
		for _, term := range searchTerms {
			results, err := searchMetArt(term)
			if err != nil {
				logger.Warnf("MetArt search failed for term %s: %v", term, err)
				continue
			}
			allResults = append(allResults, results...)
		}
	case "metartx":
		for _, term := range searchTerms {
			results, err := searchMetartX(term)
			if err != nil {
				logger.Warnf("MetartX search failed for term %s: %v", term, err)
				continue
			}
			allResults = append(allResults, results...)
		}
	case "playboy":
		for _, term := range searchTerms {
			results, err := searchPlayboy(term)
			if err != nil {
				logger.Warnf("Playboy search failed for term %s: %v", term, err)
				continue
			}
			allResults = append(allResults, results...)
		}
	case "playboyplus":
		for _, term := range searchTerms {
			results, err := searchPlayboyPlusByModel(term)
			if err != nil {
				logger.Warnf("PlayboyPlus search failed for term %s: %v", term, err)
				continue
			}
			allResults = append(allResults, results...)
		}
	case "vixen":
		for _, term := range searchTerms {
			results, err := searchVixen(term)
			if err != nil {
				logger.Warnf("Vixen search failed for term %s: %v", term, err)
				continue
			}
			allResults = append(allResults, results...)
		}
	case "sexart":
		for _, term := range searchTerms {
			results, err := searchSexArt(term)
			if err != nil {
				logger.Warnf("SexArt search failed for term %s: %v", term, err)
				continue
			}
			allResults = append(allResults, results...)
		}
	case "lifeerotic":
		for _, term := range searchTerms {
			results, err := searchLifeErotic(term)
			if err != nil {
				logger.Warnf("LifeErotic search failed for term %s: %v", term, err)
				continue
			}
			allResults = append(allResults, results...)
		}
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}

	seen := make(map[string]bool)
	uniqueResults := make([]GallerySearchResult, 0)
	for _, r := range allResults {
		key := r.URL
		if r.SourceID != "" {
			key = r.Provider + ":" + r.SourceID
		}
		if !seen[key] {
			seen[key] = true
			// Filter out non-gallery content
			if isGalleryResult(r) {
				uniqueResults = append(uniqueResults, r)
			} else {
				logger.Debugf("Filtered out non-gallery result: %s (%s)", r.Title, r.URL)
			}
		}
	}

	var existingGalleries []models.Gallery
	database.DB.Find(&existingGalleries)

	galleryNames := make(map[string]models.Gallery)
	for _, g := range existingGalleries {
		normalizedName := normalizeGalleryName(g.Name)
		if normalizedName != "" {
			galleryNames[normalizedName] = g
		}
	}

	type galleryStatus int
	const (
		statusMissing galleryStatus = iota
		statusUnsure
		statusExisting
	)

	type resultWithStatus struct {
		result    GallerySearchResult
		galleryID uint
		status    galleryStatus
	}

	var resultsWithStatus []resultWithStatus
	existingCount := 0
	unsureCount := 0

	for _, result := range uniqueResults {
		normalizedResultName := normalizeGalleryName(result.Title)

		if existingGallery, found := galleryNames[normalizedResultName]; found {
			resultWithID := result
			resultWithID.ID = existingGallery.ID

			if existingGallery.Provider != "" && strings.EqualFold(existingGallery.Provider, provider) {
				resultsWithStatus = append(resultsWithStatus, resultWithStatus{result: resultWithID, galleryID: existingGallery.ID, status: statusExisting})
				existingCount++
			} else {
				resultsWithStatus = append(resultsWithStatus, resultWithStatus{result: resultWithID, galleryID: existingGallery.ID, status: statusUnsure})
				unsureCount++
			}
		} else {
			resultsWithStatus = append(resultsWithStatus, resultWithStatus{result: result, galleryID: 0, status: statusMissing})
		}
	}

	var missingGalleries []GallerySearchResult
	var unsureGalleries []GallerySearchResult

	for _, rs := range resultsWithStatus {
		if rs.status == statusMissing {
			missingGalleries = append(missingGalleries, rs.result)
		} else if rs.status == statusUnsure {
			unsureGalleries = append(unsureGalleries, rs.result)
		}
	}

	logger.Infof("[ScanSource] %s for %s: found=%d, existing=%d, unsure=%d, missing=%d",
		provider, person.Name, len(uniqueResults), existingCount, unsureCount, len(missingGalleries))

	return &PersonScanResult{
		PersonID:         personID,
		PersonName:       person.Name,
		Provider:         provider,
		FoundCount:       len(uniqueResults),
		ExistingCount:    existingCount,
		UnsureCount:      unsureCount,
		MissingCount:     len(missingGalleries),
		MissingGalleries: missingGalleries,
		UnsureGalleries:  unsureGalleries,
	}, nil
}

// isGalleryResult checks if a search result is an actual gallery and not a video/clip or profile page
func isGalleryResult(result GallerySearchResult) bool {
	lowerURL := strings.ToLower(result.URL)
	lowerTitle := strings.ToLower(result.Title)

	// Filter out video/clip content
	videoKeywords := []string{"/videos/", "/clips/", "/video/", "/clip/", "video", "scene"}
	for _, keyword := range videoKeywords {
		if strings.Contains(lowerURL, keyword) || strings.Contains(lowerTitle, keyword) {
			return false
		}
	}

	// Filter out profile/model pages - these typically don't have gallery-like URLs
	profileKeywords := []string{"/model/", "/models/", "/performer/", "/performers/", "/actor/", "/actors/", "/profile/"}
	for _, keyword := range profileKeywords {
		// Only filter if it's just a model profile, not a gallery under a model
		// e.g., /model/libby/ is a profile, but /model/libby/gallery/... is a gallery
		if strings.Contains(lowerURL, keyword) {
			// Check if this is actually a gallery page (has /gallery/ in the path)
			if !strings.Contains(lowerURL, "/gallery/") {
				return false
			}
		}
	}

	// Filter out pages that are clearly not galleries (tag pages, search pages, etc.)
	excludePatterns := []string{"/tags/", "/categories/", "/search", "/browse"}
	for _, pattern := range excludePatterns {
		if strings.Contains(lowerURL, pattern) {
			return false
		}
	}

	return true
}

func normalizeGalleryName(name string) string {
	name = strings.ToLower(name)
	name = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return r
		}
		return -1
	}, name)
	return strings.TrimSpace(name)
}

// scrapePlayboyPlusGallery attempts to scrape metadata for a PlayboyPlus gallery
func scrapePlayboyPlusGallery(urlStr string) (*GalleryMetadata, error) {
	// Since direct scraping often redirects to join page for non-logged users,
	// we use Algolia to find the record by its ObjectID (which is at the end of the URL)
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid PlayboyPlus URL: %w", err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid PlayboyPlus URL format")
	}
	objectID := parts[len(parts)-1]

	// Query Algolia for this specific object
	appID := "TSMKFA364Q"
	apiKey := "MWU2MzkyY2ZhNzdhZDA1MzFjNDFjNTRhYjczYTM2MDNlNTQ5Yzc0NGE2MzYzYWVkZTQyYzJiYWNhYzU0ZDhkN3ZhbGlkVW50aWw9MTc2OTU2MTU1NSZyZXN0cmljdEluZGljZXM9YWxsJTJBJmZpbHRlcnM9c2VnbWVudCUzQXBsYXlib3lwbHVz"
	indexName := "all_photosets"

	apiURL := fmt.Sprintf("https://%s-dsn.algolia.net/1/indexes/%s/%s", appID, indexName, objectID)

	client := GetHTTPClient(apiURL)
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("X-Algolia-Application-Id", appID)
	req.Header.Set("X-Algolia-API-Key", apiKey)
	req.Header.Set("Referer", "https://www.playboyplus.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PlayboyPlus record from Algolia: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("PlayboyPlus Algolia returned status %d for object %s", resp.StatusCode, objectID)
	}

	var hit struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		ReleaseDate string `json:"release_date"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&hit); err != nil {
		return nil, fmt.Errorf("failed to decode PlayboyPlus Algolia record: %w", err)
	}

	metadata := &GalleryMetadata{
		Provider:    "PlayboyPlus",
		SourceURL:   urlStr,
		Description: hit.Description,
	}

	if hit.ReleaseDate != "" {
		if parsed, err := time.Parse("2006-01-02", hit.ReleaseDate[:10]); err == nil {
			metadata.ReleaseDate = parsed
		}
	}

	return metadata, nil
}
