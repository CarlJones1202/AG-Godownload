package services

import (
	"encoding/json"
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
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
	Provider     string    `json:"provider"`
	Description  string    `json:"description"`
	Rating       float64   `json:"rating"`
	ReleaseDate  time.Time `json:"release_date"`
	SourceURL    string    `json:"source_url"`
	ThumbnailURL string    `json:"thumbnail_url"`
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

	// Search EternalDesire
	eternaldesireResults, err := searchEternalDesire(searchQuery)
	if err != nil {
		logger.Warnf("EternalDesire search failed: %v", err)
	} else {
		results = append(results, eternaldesireResults...)
	}

	// Search MPLStudios
	mplResults, err := searchMPLStudios(searchQuery)
	if err != nil {
		logger.Warnf("MPLStudios search failed: %v", err)
	} else {
		results = append(results, mplResults...)
	}

	// Search VivThomas
	vivResults, err := searchVivThomas(searchQuery)
	if err != nil {
		logger.Warnf("VivThomas search failed: %v", err)
	} else {
		results = append(results, vivResults...)
	}

	// Search WowGirls
	wowResults, err := searchWowGirls(searchQuery)
	if err != nil {
		logger.Warnf("WowGirls search failed: %v", err)
	} else {
		results = append(results, wowResults...)
	}

	// Search RylskyArt
	rylskyartResults, err := searchRylskyArt(searchQuery)
	if err != nil {
		logger.Warnf("RylskyArt search failed: %v", err)
	} else {
		results = append(results, rylskyartResults...)
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
	case "eternaldesire":
		return scrapeEternalDesireGallery(sourceURL, sourceID)
	case "mplstudios":
		return scrapeMPLStudiosGallery(sourceURL, sourceID)
	case "vivthomas":
		return scrapeVivThomasGallery(sourceURL, sourceID)
	case "wowgirls":
		return scrapeWowGirlsGallery(sourceURL, sourceID)
	case "rylskyart":
		return scrapeRylskyArtGallery(sourceURL, sourceID)
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

// searchRylskyArt searches RylskyArt for matching galleries using their internal API
func searchRylskyArt(query string) ([]GallerySearchResult, error) {
	apiURL := fmt.Sprintf("https://www.rylskyart.com/api/search-results?searchPhrase=%s&page=1&pageSize=30&sortBy=latest-gallery", url.QueryEscape(query))
	logger.Infof("[RylskyArt] Searching API: %s", apiURL)

	client := GetHTTPClient(apiURL)
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search RylskyArt API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("RylskyArt API returned status %d", resp.StatusCode)
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
		logger.Errorf("[RylskyArt] Failed to parse JSON: %v", err)
		return nil, fmt.Errorf("failed to parse API JSON")
	}

	var results []GallerySearchResult
	for _, entry := range apiResp.Items {
		item := entry.Item

		if !strings.Contains(strings.ToLower(item.Path), "/gallery/") {
			continue
		}

		galleryURL := "https://www.rylskyart.com" + item.Path
		thumbURL := "https://www.rylskyart.com" + item.Thumbnail

		dateStr := item.PublishedAt
		if len(dateStr) > 10 {
			dateStr = dateStr[:10]
		}

		results = append(results, GallerySearchResult{
			Provider:    "RylskyArt",
			Title:       item.Name,
			URL:         galleryURL,
			Thumbnail:   thumbURL,
			ReleaseDate: dateStr,
		})
	}

	logger.Infof("[RylskyArt] Found %d results via API", len(results))
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
		Name                string  `json:"name"`
		Description         string  `json:"description"`
		RatingAverage       float64 `json:"ratingAverage"`
		PublishedAt         string  `json:"publishedAt"`
		CoverImageURL       string  `json:"coverImageUrl"`
		CoverImagePath      string  `json:"coverImagePath"`
		CoverCleanImagePath string  `json:"coverCleanImagePath"`
		Cover               string  `json:"cover"`
		CoverImage          string  `json:"coverImage"`
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

	// Prefer explicit cover image fields from the API — different API versions use
	// slightly different field names. Fall back to the constructed /photo/ URL only
	// if none of the known fields are present.
	if galleryDetail.CoverCleanImagePath != "" {
		metadata.ThumbnailURL = toAbsoluteMetArtURL(galleryDetail.CoverCleanImagePath)
	} else if galleryDetail.CoverImagePath != "" {
		metadata.ThumbnailURL = toAbsoluteMetArtURL(galleryDetail.CoverImagePath)
	} else if galleryDetail.CoverImageURL != "" {
		metadata.ThumbnailURL = toAbsoluteMetArtURL(galleryDetail.CoverImageURL)
	} else if galleryDetail.CoverImage != "" {
		metadata.ThumbnailURL = toAbsoluteMetArtURL(galleryDetail.CoverImage)
	} else if galleryDetail.Cover != "" {
		metadata.ThumbnailURL = toAbsoluteMetArtURL(galleryDetail.Cover)
	} else {
		// Construct thumbnail URL from gallery name (preserve original casing)
		metadata.ThumbnailURL = fmt.Sprintf("https://www.metart.com/photo/%s/0/0.jpg", galleryName)
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

// toAbsoluteMetArtURL ensures the returned thumbnail URL is absolute and uses the metart.com domain.
func toAbsoluteMetArtURL(path string) string {
	if strings.HasPrefix(path, "http") {
		return path
	}
	// Some API fields may start with "/" or be a relative path without leading slash
	if strings.HasPrefix(path, "/") {
		return "https://www.metart.com" + path
	}
	return "https://www.metart.com/" + path
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

// searchEternalDesire searches EternalDesire for matching galleries using their internal API
func searchEternalDesire(query string) ([]GallerySearchResult, error) {
	apiURL := fmt.Sprintf("https://www.eternaldesire.com/api/search-results?searchPhrase=%s&page=1&pageSize=30&sortBy=latest-gallery", url.QueryEscape(query))
	logger.Infof("[EternalDesire] Searching API: %s", apiURL)

	client := GetHTTPClient(apiURL)
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search EternalDesire API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("EternalDesire API returned status %d", resp.StatusCode)
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
		logger.Errorf("[EternalDesire] Failed to parse JSON: %v", err)
		return nil, fmt.Errorf("failed to parse API JSON")
	}

	var results []GallerySearchResult
	for _, entry := range apiResp.Items {
		item := entry.Item

		galleryURL := "https://www.eternaldesire.com" + item.Path
		thumbURL := "https://www.eternaldesire.com" + item.Thumbnail

		dateStr := item.PublishedAt
		if len(dateStr) > 10 {
			dateStr = dateStr[:10]
		}

		results = append(results, GallerySearchResult{
			Provider:    "EternalDesire",
			Title:       item.Name,
			URL:         galleryURL,
			Thumbnail:   thumbURL,
			ReleaseDate: dateStr,
		})
	}

	logger.Infof("[EternalDesire] Found %d results via API", len(results))
	return results, nil
}

// searchVivThomas searches VivThomas for matching galleries using a simple site search and HTML parsing
func searchVivThomas(query string) ([]GallerySearchResult, error) {
	candidates := []string{
		fmt.Sprintf("https://www.vivthomas.com/?s=%s", url.QueryEscape(query)),
	}

	var results []GallerySearchResult
	for idx, apiURL := range candidates {
		logger.Infof("[VivThomas] Searching candidate %d -> %s", idx, apiURL)
		client := GetHTTPClient(apiURL)
		resp, err := client.Get(apiURL)
		if err != nil {
			logger.Warnf("[VivThomas] candidate failed: %s -> %v", apiURL, err)
			continue
		}
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != 200 {
			logger.Warnf("[VivThomas] candidate returned status %d", resp.StatusCode)
			continue
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(bodyBytes)))
		if err != nil {
			logger.Warnf("[VivThomas] failed to parse HTML from %s: %v", apiURL, err)
			continue
		}

		// Look for anchors that appear to point to galleries/sets
		doc.Find("a").Each(func(i int, s *goquery.Selection) {
			if i >= 50 {
				return
			}
			href, _ := s.Attr("href")
			text := strings.TrimSpace(s.Text())
			if href == "" {
				return
			}
			lower := strings.ToLower(href)
			// heuristics for VivThomas gallery URLs
			if strings.Contains(lower, "/gallery/") || strings.Contains(lower, "/photo-") || strings.Contains(lower, "/series/") || strings.Contains(lower, "/set/") || strings.Contains(lower, "/update/") {
				if !strings.HasPrefix(href, "http") {
					if strings.HasPrefix(href, "/") {
						href = "https://www.vivthomas.com" + href
					} else {
						href = "https://www.vivthomas.com/" + href
					}
				}
				thumb := ""
				if img := s.Find("img"); img.Length() > 0 {
					if src, ok := img.Attr("src"); ok {
						thumb = src
					}
				}
				if text == "" {
					// try to get title from img alt
					if img := s.Find("img"); img.Length() > 0 {
						if alt, ok := img.Attr("alt"); ok {
							text = strings.TrimSpace(alt)
						}
					}
				}
				if text == "" {
					text = href
				}
				results = append(results, GallerySearchResult{Provider: "VivThomas", Title: text, URL: href, Thumbnail: thumb})
			}
		})

		if len(results) > 0 {
			logger.Infof("[VivThomas] found %d results via HTML parsing", len(results))
			return results, nil
		}
	}

	return nil, fmt.Errorf("no results from VivThomas search")
}

// searchWowGirls searches WowGirls for matching galleries using simple HTML search
func searchWowGirls(query string) ([]GallerySearchResult, error) {
	candidates := []string{
		fmt.Sprintf("https://www.wowgirls.com/?s=%s", url.QueryEscape(query)),
	}

	var results []GallerySearchResult
	for idx, apiURL := range candidates {
		logger.Infof("[WowGirls] Searching candidate %d -> %s", idx, apiURL)
		client := GetHTTPClient(apiURL)
		resp, err := client.Get(apiURL)
		if err != nil {
			logger.Warnf("[WowGirls] candidate failed: %s -> %v", apiURL, err)
			continue
		}
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != 200 {
			logger.Warnf("[WowGirls] candidate returned status %d", resp.StatusCode)
			continue
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(bodyBytes)))
		if err != nil {
			logger.Warnf("[WowGirls] failed to parse HTML from %s: %v", apiURL, err)
			continue
		}

		doc.Find("a").Each(func(i int, s *goquery.Selection) {
			if i >= 50 {
				return
			}
			href, _ := s.Attr("href")
			text := strings.TrimSpace(s.Text())
			if href == "" {
				return
			}
			lower := strings.ToLower(href)
			if strings.Contains(lower, "/gallery/") || strings.Contains(lower, "/photos/") || strings.Contains(lower, "/set/") || strings.Contains(lower, "/update/") {
				if !strings.HasPrefix(href, "http") {
					if strings.HasPrefix(href, "/") {
						href = "https://www.wowgirls.com" + href
					} else {
						href = "https://www.wowgirls.com/" + href
					}
				}
				thumb := ""
				if img := s.Find("img"); img.Length() > 0 {
					if src, ok := img.Attr("src"); ok {
						thumb = src
					}
				}
				if text == "" {
					if img := s.Find("img"); img.Length() > 0 {
						if alt, ok := img.Attr("alt"); ok {
							text = strings.TrimSpace(alt)
						}
					}
				}
				if text == "" {
					text = href
				}
				results = append(results, GallerySearchResult{Provider: "WowGirls", Title: text, URL: href, Thumbnail: thumb})
			}
		})

		if len(results) > 0 {
			logger.Infof("[WowGirls] found %d results via HTML parsing", len(results))
			return results, nil
		}
	}

	return nil, fmt.Errorf("no results from WowGirls search")
}

// scrapeWowGirlsGallery scrapes metadata from a WowGirls gallery page
func scrapeWowGirlsGallery(urlStr, uuid string) (*GalleryMetadata, error) {
	client := GetHTTPClient(urlStr)
	resp, err := client.Get(urlStr)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch WowGirls gallery: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("WowGirls returned status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse WowGirls gallery: %w", err)
	}

	metadata := &GalleryMetadata{Provider: "WowGirls", SourceURL: urlStr}
	if title, ok := doc.Find("meta[property='og:title']").Attr("content"); ok {
		metadata.Description = title
	} else {
		metadata.Description = strings.TrimSpace(doc.Find("h1, .entry-title").First().Text())
	}
	if desc, ok := doc.Find("meta[property='og:description']").Attr("content"); ok {
		metadata.Description = desc
	}
	if thumb, ok := doc.Find("meta[property='og:image']").Attr("content"); ok {
		metadata.ThumbnailURL = thumb
	}
	dateText := strings.TrimSpace(doc.Find("time, .posted-on, .entry-date").First().Text())
	if dateText != "" {
		for _, layout := range []string{"January 2, 2006", "Jan 2, 2006", "2006-01-02", "02/01/2006"} {
			if parsed, err := time.Parse(layout, dateText); err == nil {
				metadata.ReleaseDate = parsed
				break
			}
		}
	}
	metadata.Rating = 0
	return metadata, nil
}

// scrapeVivThomasGallery scrapes metadata from a VivThomas gallery page
func scrapeVivThomasGallery(urlStr, uuid string) (*GalleryMetadata, error) {
	client := GetHTTPClient(urlStr)
	resp, err := client.Get(urlStr)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch VivThomas gallery: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("VivThomas returned status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse VivThomas gallery: %w", err)
	}

	metadata := &GalleryMetadata{Provider: "VivThomas", SourceURL: urlStr}

	if title, ok := doc.Find("meta[property='og:title']").Attr("content"); ok {
		metadata.Description = title
	} else {
		metadata.Description = strings.TrimSpace(doc.Find("h1, .entry-title").First().Text())
	}

	if desc, ok := doc.Find("meta[property='og:description']").Attr("content"); ok {
		metadata.Description = desc
	}

	if thumb, ok := doc.Find("meta[property='og:image']").Attr("content"); ok {
		metadata.ThumbnailURL = thumb
	}

	// Try to parse date
	dateText := strings.TrimSpace(doc.Find("time, .posted-on, .entry-date").First().Text())
	if dateText != "" {
		for _, layout := range []string{"January 2, 2006", "Jan 2, 2006", "2006-01-02", "02/01/2006"} {
			if parsed, err := time.Parse(layout, dateText); err == nil {
				metadata.ReleaseDate = parsed
				break
			}
		}
	}

	metadata.Rating = 0
	return metadata, nil
}

// scrapeRylskyArtGallery scrapes full metadata from a RylskyArt gallery using their API
func scrapeRylskyArtGallery(urlStr, uuid string) (*GalleryMetadata, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid source URL: %w", err)
	}

	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(pathParts) < 2 {
		return nil, fmt.Errorf("invalid RylskyArt URL format, cannot extract date/name")
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

	apiURL := fmt.Sprintf("https://www.rylskyart.com/api/gallery?name=%s&date=%s", galleryName, galleryDate)
	logger.Infof("[RylskyArt] Fetching detail API: %s", apiURL)

	client := GetHTTPClient(apiURL)
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch RylskyArt gallery API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("RylskyArt gallery API returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read RylskyArt response: %w", err)
	}

	var galleryDetail struct {
		Name          string  `json:"name"`
		Description   string  `json:"metaDescription"`
		RatingAverage float64 `json:"ratingAverage"`
		PublishedAt   string  `json:"publishedAt"`
	}

	if err := json.Unmarshal(bodyBytes, &galleryDetail); err != nil {
		logger.Errorf("[RylskyArt] Failed to parse detail JSON: %v", err)
		return nil, fmt.Errorf("failed to parse detail JSON")
	}

	metadata := &GalleryMetadata{
		Provider:    "RylskyArt",
		SourceURL:   urlStr,
		Description: galleryDetail.Description,
	}

	if galleryDetail.RatingAverage > 0 {
		metadata.Rating = galleryDetail.RatingAverage
	}

	if dateStr := galleryDetail.PublishedAt; dateStr != "" {
		if len(dateStr) > 10 {
			dateStr = dateStr[:10]
		}
		if parsed, err := time.Parse("2006-01-02", dateStr); err == nil {
			metadata.ReleaseDate = parsed
		}
	}

	return metadata, nil
}

// scrapeEternalDesireGallery scrapes full metadata from an EternalDesire gallery page
func scrapeEternalDesireGallery(urlStr, uuid string) (*GalleryMetadata, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid source URL: %w", err)
	}

	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(pathParts) < 2 {
		return nil, fmt.Errorf("invalid EternalDesire URL format, cannot extract date/name")
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

	apiURL := fmt.Sprintf("https://www.eternaldesire.com/api/gallery?name=%s&date=%s", galleryName, galleryDate)
	logger.Infof("[EternalDesire] Fetching detail API: %s", apiURL)

	client := GetHTTPClient(apiURL)
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch EternalDesire gallery API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("EternalDesire gallery API returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read EternalDesire response: %w", err)
	}

	var galleryDetail struct {
		Name          string  `json:"name"`
		Description   string  `json:"description"`
		RatingAverage float64 `json:"ratingAverage"`
		PublishedAt   string  `json:"publishedAt"`
	}

	if err := json.Unmarshal(bodyBytes, &galleryDetail); err != nil {
		logger.Errorf("[EternalDesire] Failed to parse detail JSON: %v", err)
		return nil, fmt.Errorf("failed to parse detail JSON")
	}

	metadata := &GalleryMetadata{
		Provider:    "EternalDesire",
		SourceURL:   urlStr,
		Description: galleryDetail.Description,
		Rating:      galleryDetail.RatingAverage,
	}

	if galleryDetail.PublishedAt != "" {
		if parsed, err := time.Parse("2006-01-02T15:04:05.000Z", galleryDetail.PublishedAt); err == nil {
			metadata.ReleaseDate = parsed
		}
	}

	logger.Infof("Scraped EternalDesire gallery: %s (Rating: %.2f)", metadata.Description[:min(50, len(metadata.Description))], metadata.Rating)
	return metadata, nil
}

// searchMPLStudios attempts to find galleries on mplstudios.com
// Enhanced with debug logging and response dumping to help diagnose
// missed results when the normal parsing doesn't pick them up.
func searchMPLStudios(query string) ([]GallerySearchResult, error) {
	// First, try the site search endpoint that returns nested arrays containing person results
	// Example: https://www.mplstudios.com/searchFor/?value={{ALIAS}}
	searchForURL := fmt.Sprintf("https://www.mplstudios.com/searchFor/?value=%s", url.QueryEscape(query))
	candidates := []string{
		searchForURL,
		fmt.Sprintf("https://www.mplstudios.com/api/search?query=%s", url.QueryEscape(query)),
		fmt.Sprintf("https://www.mplstudios.com/search?query=%s", url.QueryEscape(query)),
		fmt.Sprintf("https://www.mplstudios.com/galleries?search=%s", url.QueryEscape(query)),
	}

	var results []GallerySearchResult

	for idx, apiURL := range candidates {
		logger.Infof("[MPLStudios] candidate %d -> %s", idx, apiURL)

		client := GetHTTPClient(apiURL)
		resp, err := client.Get(apiURL)
		if err != nil {
			logger.Warnf("[MPLStudios] candidate failed: %s -> %v", apiURL, err)
			continue
		}

		bodyBytes, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			logger.Warnf("[MPLStudios] failed to read body from %s: %v", apiURL, readErr)
			continue
		}

		logger.Infof("[MPLStudios] %s returned status %d, %d bytes", apiURL, resp.StatusCode, len(bodyBytes))

		// Dump response for debugging (timestamped)
		timestamp := time.Now().Unix()
		// Choose extension based on content-type when available
		ext := "html"
		if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "application/json") {
			ext = "json"
		}
		debugName := fmt.Sprintf("debug_mplstudios_candidate_%d_%d.%s", idx, timestamp, ext)
		if err := os.WriteFile(debugName, bodyBytes, 0644); err != nil {
			logger.Warnf("[MPLStudios] failed to write debug file %s: %v", debugName, err)
		} else {
			logger.Infof("[MPLStudios] dumped response to %s", debugName)
		}

		// Quick check for common blocking/age-verification phrases
		bodyStrLower := strings.ToLower(string(bodyBytes))
		if strings.Contains(bodyStrLower, "age verification") || strings.Contains(bodyStrLower, "age gate") || strings.Contains(bodyStrLower, "verify your age") {
			logger.Warnf("[MPLStudios] candidate %d appears to be age-gated or blocked", idx)
			// continue to next candidate since this response isn't useful
			continue
		}

		// Special handling for searchFor endpoint which returns nested arrays
		if apiURL == searchForURL {
			// Try to parse JSON (it's typically an array of arrays)
			var root interface{}
			if err := json.Unmarshal(bodyBytes, &root); err == nil {
				// This endpoint frequently returns gallery search results in root[1]
				// Parse galleries directly from the searchFor response first (gallery search)
				if rootArr, ok := root.([]interface{}); ok && len(rootArr) > 1 {
					if galleryGroups, ok := rootArr[1].([]interface{}); ok && len(galleryGroups) > 0 {
						if firstGroup, ok := galleryGroups[0].([]interface{}); ok {
							for gi, entry := range firstGroup {
								if entArr, ok := entry.([]interface{}); ok && len(entArr) > 2 {
									titleRaw := fmt.Sprintf("%v", entArr[2])
									urlRaw := fmt.Sprintf("%v", entArr[1])
									thumbRaw := fmt.Sprintf("%v", entArr[0])
									titleClean := stripHTML(titleRaw)
									urlClean := urlRaw
									if !strings.HasPrefix(urlClean, "http") {
										if strings.HasPrefix(urlClean, "/") {
											urlClean = "https://www.mplstudios.com" + urlClean
										} else {
											urlClean = "https://www.mplstudios.com/" + urlClean
										}
									}
									results = append(results, GallerySearchResult{Provider: "MPLStudios", Title: titleClean, URL: urlClean, Thumbnail: thumbRaw})
									logger.Debugf("[MPLStudios] parsed gallery candidate from searchFor %d: title=%s url=%s", gi, titleClean, urlClean)
								}
							}
							if len(results) > 0 {
								logger.Infof("[MPLStudios] parsed %d galleries directly from searchFor response", len(results))
								return results, nil
							}
						}
					}
				}

				if href, name, ok := findBestPersonFromSearchFor(root, query); ok {
					logger.Infof("[MPLStudios] searchFor matched person '%s' -> %s", name, href)
					// Build full person URL if necessary
					if !strings.HasPrefix(href, "http") {
						if strings.HasPrefix(href, "/") {
							href = "https://www.mplstudios.com" + href
						} else {
							href = "https://www.mplstudios.com/" + href
						}
					}
					// Fetch the person's page and parse galleries from it
					personClient := GetHTTPClient(href)
					presp, err := personClient.Get(href)
					if err != nil {
						logger.Warnf("[MPLStudios] failed to fetch person page %s: %v", href, err)
					} else {
						pbody, _ := io.ReadAll(presp.Body)
						presp.Body.Close()
						// Dump person page for debugging
						pfname := fmt.Sprintf("debug_mplstudios_person_%d_%d.html", idx, time.Now().Unix())
						_ = os.WriteFile(pfname, pbody, 0644)
						logger.Infof("[MPLStudios] dumped person page to %s", pfname)

						// Parse person page for gallery links
						pageStr := string(pbody)

						// Try to find an embedded array literal and parse gallery candidates
						re := regexp.MustCompile(`(?s)(?:var|let|const)\s+result\s*=\s*(\[\s*\[.*?\]\s*\])\s*;`)
						match := re.FindStringSubmatch(pageStr)
						var arrayText string
						if len(match) >= 2 {
							arrayText = match[1]
						} else {
							re2 := regexp.MustCompile(`(?s)(\[\s*\[.*?\]\s*\])`)
							m2 := re2.FindStringSubmatch(pageStr)
							if len(m2) >= 2 {
								arrayText = m2[1]
							}
						}

						if arrayText != "" {
							cleaned := strings.ReplaceAll(arrayText, "\\/", "/")
							var parsed interface{}
							if err := json.Unmarshal([]byte(cleaned), &parsed); err == nil {
								if rootArr, ok := parsed.([]interface{}); ok && len(rootArr) > 1 {
									if galleryGroups, ok := rootArr[1].([]interface{}); ok && len(galleryGroups) > 0 {
										if firstGroup, ok := galleryGroups[0].([]interface{}); ok {
											for gi, entry := range firstGroup {
												if entArr, ok := entry.([]interface{}); ok && len(entArr) > 2 {
													titleRaw := fmt.Sprintf("%v", entArr[2])
													urlRaw := fmt.Sprintf("%v", entArr[1])
													thumbRaw := fmt.Sprintf("%v", entArr[0])
													titleClean := stripHTML(titleRaw)
													urlClean := urlRaw
													if !strings.HasPrefix(urlClean, "http") {
														if strings.HasPrefix(urlClean, "/") {
															urlClean = "https://www.mplstudios.com" + urlClean
														} else {
															urlClean = "https://www.mplstudios.com/" + urlClean
														}
													}
													results = append(results, GallerySearchResult{Provider: "MPLStudios", Title: titleClean, URL: urlClean, Thumbnail: thumbRaw})
													logger.Debugf("[MPLStudios] parsed gallery candidate %d: title=%s url=%s", gi, titleClean, urlClean)
												}
											}
										}
									}
								}
							} else {
								logger.Debugf("[MPLStudios] failed to JSON-decode embedded array: %v", err)
							}
						} else {
							logger.Debugf("[MPLStudios] no embedded array found on person page; falling back to DOM selectors")
						}

						// DOM fallback
						doc, err := goquery.NewDocumentFromReader(strings.NewReader(pageStr))
						if err == nil {
							doc.Find("a[href*='/gallery/'], a[href*='/galleries/']").Each(func(i int, s *goquery.Selection) {
								href2, _ := s.Attr("href")
								text := strings.TrimSpace(s.Text())
								thumb := ""
								if img := s.Find("img"); img.Length() > 0 {
									if src, ok := img.Attr("src"); ok {
										thumb = src
									}
								}
								if href2 != "" {
									if !strings.HasPrefix(href2, "http") {
										if strings.HasPrefix(href2, "/") {
											href2 = "https://www.mplstudios.com" + href2
										} else {
											href2 = "https://www.mplstudios.com/" + href2
										}
									}
									results = append(results, GallerySearchResult{Provider: "MPLStudios", Title: text, URL: href2, Thumbnail: thumb})
								}
							})
							if len(results) > 0 {
								logger.Infof("[MPLStudios] found %d galleries on person page via DOM fallback", len(results))
								return results, nil
							}
						}
						logger.Warnf("[MPLStudios] no galleries found on person page %s", href)
					}
				} else {
					logger.Debugf("[MPLStudios] searchFor did not find a person match for '%s'; attempting to parse galleries directly from searchFor response", query)
					// The searchFor response can include galleries in the second element
					if rootArr, ok := root.([]interface{}); ok && len(rootArr) > 1 {
						if galleryGroups, ok := rootArr[1].([]interface{}); ok && len(galleryGroups) > 0 {
							if firstGroup, ok := galleryGroups[0].([]interface{}); ok {
								for gi, entry := range firstGroup {
									if entArr, ok := entry.([]interface{}); ok && len(entArr) > 2 {
										titleRaw := fmt.Sprintf("%v", entArr[2])
										urlRaw := fmt.Sprintf("%v", entArr[1])
										thumbRaw := fmt.Sprintf("%v", entArr[0])
										titleClean := stripHTML(titleRaw)
										urlClean := urlRaw
										if !strings.HasPrefix(urlClean, "http") {
											if strings.HasPrefix(urlClean, "/") {
												urlClean = "https://www.mplstudios.com" + urlClean
											} else {
												urlClean = "https://www.mplstudios.com/" + urlClean
											}
										}
										results = append(results, GallerySearchResult{Provider: "MPLStudios", Title: titleClean, URL: urlClean, Thumbnail: thumbRaw})
										logger.Debugf("[MPLStudios] parsed gallery candidate from searchFor %d: title=%s url=%s", gi, titleClean, urlClean)
									}
								}
								if len(results) > 0 {
									logger.Infof("[MPLStudios] parsed %d galleries directly from searchFor response", len(results))
									return results, nil
								}
							}
						}
					}
					logger.Debugf("[MPLStudios] no direct galleries found in searchFor response for '%s'", query)
				}
			} else {
				logger.Debugf("[MPLStudios] searchFor response not JSON: %v", err)

				// Attempt to extract JSON from an HTML wrapper (e.g. <pre> or JS assignment)
				if cleaned, exErr := extractJSONFromHTML(bodyBytes); exErr == nil {
					// MPL often escapes slashes as \\/ in their JSON blobs
					cleanedStr := strings.ReplaceAll(string(cleaned), "\\/", "/")
					if err2 := json.Unmarshal([]byte(cleanedStr), &root); err2 == nil {
						if href, name, ok := findBestPersonFromSearchFor(root, query); ok {
							logger.Infof("[MPLStudios] searchFor matched person '%s' -> %s", name, href)
							if !strings.HasPrefix(href, "http") {
								if strings.HasPrefix(href, "/") {
									href = "https://www.mplstudios.com" + href
								} else {
									href = "https://www.mplstudios.com/" + href
								}
							}
							personClient := GetHTTPClient(href)
							presp, err := personClient.Get(href)
							if err != nil {
								logger.Warnf("[MPLStudios] failed to fetch person page %s: %v", href, err)
							} else {
								pbody, _ := io.ReadAll(presp.Body)
								presp.Body.Close()
								pfname := fmt.Sprintf("debug_mplstudios_person_%d_%d.html", idx, time.Now().Unix())
								_ = os.WriteFile(pfname, pbody, 0644)
								logger.Infof("[MPLStudios] dumped person page to %s", pfname)

								// Reuse existing person-page parsing logic
								pageStr := string(pbody)
								re := regexp.MustCompile(`(?s)(?:var|let|const)\s+result\s*=\s*(\[\s*\[.*?\]\s*\])\s*;`)
								match := re.FindStringSubmatch(pageStr)
								var arrayText string
								if len(match) >= 2 {
									arrayText = match[1]
								} else {
									re2 := regexp.MustCompile(`(?s)(\[\s*\[.*?\]\s*\])`)
									m2 := re2.FindStringSubmatch(pageStr)
									if len(m2) >= 2 {
										arrayText = m2[1]
									}
								}

								if arrayText != "" {
									cleanedArr := strings.ReplaceAll(arrayText, "\\/", "/")
									var parsed interface{}
									if err := json.Unmarshal([]byte(cleanedArr), &parsed); err == nil {
										if rootArr, ok := parsed.([]interface{}); ok && len(rootArr) > 1 {
											if galleryGroups, ok := rootArr[1].([]interface{}); ok && len(galleryGroups) > 0 {
												if firstGroup, ok := galleryGroups[0].([]interface{}); ok {
													for gi, entry := range firstGroup {
														if entArr, ok := entry.([]interface{}); ok && len(entArr) > 2 {
															titleRaw := fmt.Sprintf("%v", entArr[2])
															urlRaw := fmt.Sprintf("%v", entArr[1])
															thumbRaw := fmt.Sprintf("%v", entArr[0])
															titleClean := stripHTML(titleRaw)
															urlClean := urlRaw
															if !strings.HasPrefix(urlClean, "http") {
																urlClean = "https://www.mplstudios.com" + urlClean
															}
															results = append(results, GallerySearchResult{Provider: "MPLStudios", Title: titleClean, URL: urlClean, Thumbnail: thumbRaw})
															logger.Debugf("[MPLStudios] parsed gallery candidate %d: title=%s url=%s", gi, titleClean, urlClean)
														}
													}
												}
											}
										}
									} else {
										logger.Debugf("[MPLStudios] failed to JSON-decode embedded array after extraction: %v", err)
									}
								} else {
									logger.Debugf("[MPLStudios] no embedded array found on person page; falling back to DOM selectors")
								}

								// DOM fallback
								doc, err := goquery.NewDocumentFromReader(strings.NewReader(pageStr))
								if err == nil {
									doc.Find("a[href*='/gallery/'], a[href*='/galleries/']").Each(func(i int, s *goquery.Selection) {
										href2, _ := s.Attr("href")
										text := strings.TrimSpace(s.Text())
										thumb := ""
										if img := s.Find("img"); img.Length() > 0 {
											if src, ok := img.Attr("src"); ok {
												thumb = src
											}
										}
										if href2 != "" {
											if !strings.HasPrefix(href2, "http") {
												href2 = "https://www.mplstudios.com" + href2
											}
											results = append(results, GallerySearchResult{Provider: "MPLStudios", Title: text, URL: href2, Thumbnail: thumb})
										}
									})
									if len(results) > 0 {
										logger.Infof("[MPLStudios] found %d galleries on person page via DOM fallback", len(results))
										return results, nil
									}
								}
								logger.Warnf("[MPLStudios] no galleries found on person page %s", href)
							}
						}
					} else {
						logger.Debugf("[MPLStudios] failed to parse extracted JSON from searchFor: %v", err2)
					}
				} else {
					logger.Debugf("[MPLStudios] extractJSONFromHTML failed: %v", exErr)
				}
			}
			// Continue to other candidates if person lookup didn't yield galleries
		}

		// If JSON, try to parse expected shapes
		var js map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &js); err == nil {
			logger.Debugf("[MPLStudios] parsed JSON keys: %v", keysOf(js))
			// Try common shapes: data.items or results
			found := 0
			if data, ok := js["data"].(map[string]interface{}); ok {
				if items, ok := data["items"].([]interface{}); ok {
					for _, it := range items {
						if m, ok := it.(map[string]interface{}); ok {
							title := fmt.Sprintf("%v", m["title"])
							path := fmt.Sprintf("%v", m["url"])
							thumb := fmt.Sprintf("%v", m["thumbnail"])
							date := fmt.Sprintf("%v", m["release_date"])
							if !strings.HasPrefix(path, "http") {
								path = "https://www.mplstudios.com" + path
							}
							results = append(results, GallerySearchResult{Provider: "MPLStudios", Title: title, URL: path, Thumbnail: thumb, ReleaseDate: date})
							found++
						}
					}
				}
			}
			// Some endpoints return {results: [...]}
			if resultsArr, ok := js["results"].([]interface{}); ok {
				for _, it := range resultsArr {
					if m, ok := it.(map[string]interface{}); ok {
						title := fmt.Sprintf("%v", m["title"])
						path := fmt.Sprintf("%v", m["url"])
						thumb := fmt.Sprintf("%v", m["thumbnail"])
						date := fmt.Sprintf("%v", m["release_date"])
						if !strings.HasPrefix(path, "http") {
							path = "https://www.mplstudios.com" + path
						}
						results = append(results, GallerySearchResult{Provider: "MPLStudios", Title: title, URL: path, Thumbnail: thumb, ReleaseDate: date})
						found++
					}
				}
			}

			logger.Infof("[MPLStudios] JSON parsing found %d items in candidate %d", found, idx)
			if len(results) > 0 {
				return results, nil
			}
		} else {
			logger.Debugf("[MPLStudios] response from %s is not JSON: %v", apiURL, err)
		}

		// Fallback: parse HTML for links
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(bodyBytes)))
		if err != nil {
			logger.Warnf("[MPLStudios] failed to parse HTML from %s: %v", apiURL, err)
			continue
		}

		anchors := doc.Find("a[href*='/gallery/'], a[href*='/galleries/']")
		logger.Infof("[MPLStudios] candidate %d: found %d anchor(s) matching gallery pattern", idx, anchors.Length())
		// Log first few hrefs for inspection
		anchors.EachWithBreak(func(i int, s *goquery.Selection) bool {
			if i >= 10 {
				return false
			}
			href, _ := s.Attr("href")
			text := strings.TrimSpace(s.Text())
			logger.Debugf("[MPLStudios] anchor %d -> href=%s text=%s", i, href, text)
			return true
		})

		anchors.Each(func(i int, s *goquery.Selection) {
			href, _ := s.Attr("href")
			text := strings.TrimSpace(s.Text())
			// thumbnail may be in img child
			thumb := ""
			if img := s.Find("img"); img.Length() > 0 {
				if src, ok := img.Attr("src"); ok {
					thumb = src
				}
			}
			if href != "" {
				if !strings.HasPrefix(href, "http") {
					if strings.HasPrefix(href, "/") {
						href = "https://www.mplstudios.com" + href
					} else {
						href = "https://www.mplstudios.com/" + href
					}
				}
				results = append(results, GallerySearchResult{Provider: "MPLStudios", Title: text, URL: href, Thumbnail: thumb})
			}
		})

		if len(results) > 0 {
			logger.Infof("[MPLStudios] candidate %d produced %d HTML results", idx, len(results))
			return results, nil
		}
	}

	return nil, fmt.Errorf("no results from MPLStudios search")
}

// findBestPersonFromSearchFor inspects the nested array response from /searchFor/
// and returns the href and display name for the best matching person if found.
// The response is often an array of arrays where each inner item contains
// ["type", "name", "url", ...] or similar shapes. We search for values
// that look like person entries and pick the one whose name best matches alias.
func findBestPersonFromSearchFor(root interface{}, alias string) (href string, name string, ok bool) {
	aliasLower := strings.ToLower(alias)
	var candidates []struct {
		href  string
		name  string
		score int
	}

	// Walk the structure recursively looking for small arrays/objects that contain a URL and a name
	var walk func(node interface{})
	walk = func(node interface{}) {
		switch v := node.(type) {
		case []interface{}:
			// If this looks like a leaf array with strings, try to interpret
			if len(v) >= 3 {
				// Collect string tokens
				strs := make([]string, 0, len(v))
				for _, it := range v {
					if s, ok := it.(string); ok {
						strs = append(strs, s)
					}
				}
				if len(strs) >= 2 {
					// Heuristics: one token looks like a url (contains "/") and another is a name (contains space or letters)
					var candidateHref, candidateName string
					for _, t := range strs {
						if strings.Contains(t, "/") && (strings.HasPrefix(t, "http") || strings.HasPrefix(t, "/")) {
							candidateHref = t
						} else if len(t) > 0 && (strings.Contains(t, " ") || unicode.IsLetter(rune(t[0]))) {
							candidateName = t
						}
					}
					if candidateHref != "" && candidateName != "" {
						score := 0
						lname := strings.ToLower(candidateName)
						if strings.EqualFold(candidateName, alias) || strings.Contains(aliasLower, strings.ToLower(candidateName)) || strings.Contains(lname, aliasLower) {
							score += 10
						}
						// fuzzy length similarity
						if levenshteinLenClose(aliasLower, strings.ToLower(candidateName)) {
							score += 5
						}
						candidates = append(candidates, struct {
							href, name string
							score      int
						}{candidateHref, candidateName, score})
					}
				}
			}
			// Recurse into children
			for _, it := range v {
				walk(it)
			}
		case map[string]interface{}:
			// Look for common keys
			if n, ok := v["name"].(string); ok {
				if u, ok2 := v["url"].(string); ok2 {
					score := 0
					lname := strings.ToLower(n)
					if strings.Contains(lname, aliasLower) || strings.EqualFold(n, alias) {
						score += 10
					}
					candidates = append(candidates, struct {
						href, name string
						score      int
					}{u, n, score})
				}
			}
			for _, it := range v {
				walk(it)
			}
		}
	}
	walk(root)

	// Choose highest score, break ties by shortest name distance
	bestScore := 0
	bestIdx := -1
	for i, c := range candidates {
		if c.score > bestScore {
			bestScore = c.score
			bestIdx = i
		}
	}
	if bestIdx >= 0 {
		return candidates[bestIdx].href, candidates[bestIdx].name, true
	}
	return "", "", false
}

// levenshteinLenClose is a lightweight heuristic: return true if lengths are within 3 and
// there are at least 2 matching starting characters. This avoids importing heavy libs.
func levenshteinLenClose(a, b string) bool {
	if abs(len(a)-len(b)) > 3 {
		return false
	}
	minLen := 2
	if len(a) < minLen || len(b) < minLen {
		return false
	}
	return a[:2] == b[:2]
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// stripHTML removes simple HTML tags and entity fragments from a string.
// It's lightweight and intended for cleaning small fragments like "Monika May</a>".
func stripHTML(s string) string {
	// remove simple tags
	re := regexp.MustCompile(`<[^>]+>`)
	cleaned := re.ReplaceAllString(s, "")
	// unescape common entities (basic)
	cleaned = strings.ReplaceAll(cleaned, "&amp;", "&")
	cleaned = strings.ReplaceAll(cleaned, "&lt;", "<")
	cleaned = strings.ReplaceAll(cleaned, "&gt;", ">")
	return strings.TrimSpace(cleaned)
}

// extractJSONFromHTML attempts to pull a JSON array/object out of an HTML wrapper.
// It looks for common patterns: a <pre> block containing raw JSON, a JS assignment
// like `var x = [...]`, and a fallback that finds the first '{' or '[' and
// returns a balanced JSON chunk.
func extractJSONFromHTML(body []byte) ([]byte, error) {
	s := string(body)

	// 1) <pre>...</pre>
	rePre := regexp.MustCompile(`(?is)<pre[^>]*>(.*?)</pre>`)
	if m := rePre.FindStringSubmatch(s); len(m) >= 2 {
		return []byte(html.UnescapeString(m[1])), nil
	}

	// 2) assignment like: var parsed = [...] or window.__DATA__ = {...}
	reAssign := regexp.MustCompile(`(?is)(?:var|let|const|window\.[\w_]+|window\['[\w_]+'\])\s*=\s*([\[{][\s\S]*?[\]}])\s*;`)
	if m := reAssign.FindStringSubmatch(s); len(m) >= 2 {
		return []byte(m[1]), nil
	}

	// 3) fallback: find first '{' or '[' and return balanced chunk
	startIdx := -1
	var open, close byte
	for i := 0; i < len(s); i++ {
		if s[i] == '{' || s[i] == '[' {
			startIdx = i
			if s[i] == '{' {
				open = '{'
				close = '}'
			} else {
				open = '['
				close = ']'
			}
			break
		}
	}
	if startIdx == -1 {
		return nil, fmt.Errorf("no JSON start char found in HTML")
	}

	depth := 0
	inString := false
	esc := false
	for i := startIdx; i < len(s); i++ {
		c := s[i]
		if esc {
			esc = false
			continue
		}
		if c == '\\' {
			esc = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if c == open {
			depth++
		} else if c == close {
			depth--
			if depth == 0 {
				return []byte(s[startIdx : i+1]), nil
			}
		}
	}

	return nil, fmt.Errorf("unbalanced JSON in HTML")
}

// keysOf returns the top-level keys of a map[string]interface{} for logging
func keysOf(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// scrapeMPLStudiosGallery scrapes metadata from an MPLStudios gallery page
func scrapeMPLStudiosGallery(urlStr, uuid string) (*GalleryMetadata, error) {
	client := GetHTTPClient(urlStr)
	resp, err := client.Get(urlStr)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch MPLStudios gallery: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("MPLStudios returned status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MPLStudios gallery: %w", err)
	}

	metadata := &GalleryMetadata{Provider: "MPLStudios", SourceURL: urlStr}

	// Prefer OpenGraph/meta tags
	if title, ok := doc.Find("meta[property='og:title']").Attr("content"); ok {
		metadata.Description = title
	} else {
		metadata.Description = strings.TrimSpace(doc.Find("h1, .title").First().Text())
	}

	if desc, ok := doc.Find("meta[property='og:description']").Attr("content"); ok {
		metadata.Description = desc
	}

	if thumb, ok := doc.Find("meta[property='og:image']").Attr("content"); ok {
		metadata.ThumbnailURL = thumb
	}

	// Try to get release date from meta or visible elements
	if dateStr, ok := doc.Find("meta[name='date']").Attr("content"); ok {
		if parsed, err := time.Parse("2006-01-02", dateStr); err == nil {
			metadata.ReleaseDate = parsed
		}
	} else {
		// Look for date text
		dateText := strings.TrimSpace(doc.Find(".date, .publish-date, time").First().Text())
		if dateText != "" {
			for _, layout := range []string{"2006-01-02", "January 2, 2006", "Jan 2, 2006", "02/01/2006"} {
				if parsed, err := time.Parse(layout, dateText); err == nil {
					metadata.ReleaseDate = parsed
					break
				}
			}
		}
	}

	// No rating on MPL; set to 0
	metadata.Rating = 0

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
	apiKey := "MDJmMzNkZTQ5YzY1NGFkOGY5NDU1OTU5M2Y4ZGFhNDdiZDM4N2QwZjY1ZWNmODkyOWRlNzE0NjRlNTVmYzNhNnZhbGlkVW50aWw9MTc3MjIzNjk3OCZyZXN0cmljdEluZGljZXM9YWxsJTJBJmZpbHRlcnM9c2VnbWVudCUzQXBsYXlib3lwbHVz"
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
	apiKey := "MDJmMzNkZTQ5YzY1NGFkOGY5NDU1OTU5M2Y4ZGFhNDdiZDM4N2QwZjY1ZWNmODkyOWRlNzE0NjRlNTVmYzNhNnZhbGlkVW50aWw9MTc3MjIzNjk3OCZyZXN0cmljdEluZGljZXM9YWxsJTJBJmZpbHRlcnM9c2VnbWVudCUzQXBsYXlib3lwbHVz"

	// Step 1: Search for the model in all_actors index
	actorsURL := fmt.Sprintf("https://%s-dsn.algolia.net/1/indexes/all_actors/query", appID)

	actorsParams := map[string]interface{}{
		"params": fmt.Sprintf("query=%s&hitsPerPage=10", url.QueryEscape(modelName)),
	}

	actorsBody, err := json.Marshal(actorsParams)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal actors query: %w", err)
	}

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

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("PlayboyPlus actors search returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read actors response: %w", err)
	}

	var actorsResp struct {
		Hits []struct {
			ObjectID string `json:"objectID"`
			Name     string `json:"name"`
			URLName  string `json:"urlName"`
		} `json:"hits"`
	}

	if err := json.Unmarshal(bodyBytes, &actorsResp); err != nil {
		return nil, fmt.Errorf("failed to decode PlayboyPlus actors response: %w", err)
	}

	// Find exact match
	var actorID string
	for _, actor := range actorsResp.Hits {
		// Case-insensitive exact match
		if strings.EqualFold(actor.Name, modelName) {
			actorID = actor.ObjectID
			break
		}
	}

	if actorID == "" {
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

	bodyBytes2, err := io.ReadAll(resp2.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read galleries response: %w", err)
	}

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

	var results []GallerySearchResult
	for _, hit := range galleriesResp.Hits {
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
	case "eternaldesire":
		for _, term := range searchTerms {
			results, err := searchEternalDesire(term)
			if err != nil {
				logger.Warnf("EternalDesire search failed for term %s: %v", term, err)
				continue
			}
			allResults = append(allResults, results...)
		}
	case "mplstudios":
		for _, term := range searchTerms {
			results, err := searchMPLStudios(term)
			if err != nil {
				logger.Warnf("MPLStudios search failed for term %s: %v", term, err)
				continue
			}
			allResults = append(allResults, results...)
		}
	case "vivthomas":
		for _, term := range searchTerms {
			results, err := searchVivThomas(term)
			if err != nil {
				logger.Warnf("VivThomas search failed for term %s: %v", term, err)
				continue
			}
			allResults = append(allResults, results...)
		}
	case "wowgirls":
		for _, term := range searchTerms {
			results, err := searchWowGirls(term)
			if err != nil {
				logger.Warnf("WowGirls search failed for term %s: %v", term, err)
				continue
			}
			allResults = append(allResults, results...)
		}
	case "rylskyart":
		for _, term := range searchTerms {
			results, err := searchRylskyArt(term)
			if err != nil {
				logger.Warnf("RylskyArt search failed for term %s: %v", term, err)
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

	// Query exclusions for this person and provider
	var exclusions []models.ScanResultExclusion
	if err := database.DB.Where("person_id = ? AND provider = ?", personID, provider).Find(&exclusions).Error; err != nil {
		logger.Warnf("Failed to query exclusions for person %d, provider %s: %v", personID, provider, err)
		// Continue anyway - exclusions are a nice-to-have filter
		exclusions = []models.ScanResultExclusion{}
	}

	// Build set of excluded SourceIDs and SourceURLs for fast lookup
	excludedSourceIDs := make(map[string]bool)
	excludedSourceURLs := make(map[string]bool)
	for _, exclusion := range exclusions {
		if exclusion.SourceID != "" {
			excludedSourceIDs[exclusion.SourceID] = true
		}
		if exclusion.SourceURL != "" {
			excludedSourceURLs[exclusion.SourceURL] = true
		}
	}

	var missingGalleries []GallerySearchResult
	var unsureGalleries []GallerySearchResult

	for _, rs := range resultsWithStatus {
		// Skip if this result matches an exclusion by SourceID or SourceURL
		isExcluded := (rs.result.SourceID != "" && excludedSourceIDs[rs.result.SourceID]) ||
			(rs.result.URL != "" && excludedSourceURLs[rs.result.URL])
		if isExcluded {
			logger.Debugf("Skipping excluded gallery: %s (SourceID: %s, URL: %s)", rs.result.Title, rs.result.SourceID, rs.result.URL)
			// Decrement counts since we're filtering this out
			if rs.status == statusExisting {
				existingCount--
			} else if rs.status == statusUnsure {
				unsureCount--
			}
			continue
		}

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
	apiKey := "MDJmMzNkZTQ5YzY1NGFkOGY5NDU1OTU5M2Y4ZGFhNDdiZDM4N2QwZjY1ZWNmODkyOWRlNzE0NjRlNTVmYzNhNnZhbGlkVW50aWw9MTc3MjIzNjk3OCZyZXN0cmljdEluZGljZXM9YWxsJTJBJmZpbHRlcnM9c2VnbWVudCUzQXBsYXlib3lwbHVz"
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
