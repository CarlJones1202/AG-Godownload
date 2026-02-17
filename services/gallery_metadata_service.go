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
	apiKey := "MWU2MzkyY2ZhNzdhZDA1MzFjNDFjNTRhYjczYTM2MDNlNTQ5Yzc0NGE2MzYzYWVkZTQyYzJiYWNhYzU0ZDhkN3ZhbGlkVW50aWw9MTc2OTU2MTU1NSZyZXN0cmljdEluZGljZXM9YWxsJTJBJmZpbHRlcnM9c2VnbWVudCUzQXBsYXlib3lwbHVz"
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
			results, err := searchPlayboyPlus(term)
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
			uniqueResults = append(uniqueResults, r)
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
