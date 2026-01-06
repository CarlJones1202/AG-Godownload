package services

import (
	"encoding/json"
	"fmt"
	"gallery_api/logger"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// GallerySearchResult represents a search result candidate
type GallerySearchResult struct {
	Provider    string `json:"provider"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Thumbnail   string `json:"thumbnail"`
	ReleaseDate string `json:"release_date"`
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

	// Search Playboy
	playboyResults, err := searchPlayboy(searchQuery)
	if err != nil {
		logger.Warnf("Playboy search failed: %v", err)
	} else {
		results = append(results, playboyResults...)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no matching galleries found")
	}

	logger.Infof("Found %d matching galleries", len(results))
	return results, nil
}

// ScrapeGalleryMetadata scrapes full metadata from a confirmed gallery URL
func ScrapeGalleryMetadata(sourceURL string, provider string) (*GalleryMetadata, error) {
	logger.Infof("Scraping metadata from %s (%s)", sourceURL, provider)

	switch strings.ToLower(provider) {
	case "metart":
		return scrapeMetArtGallery(sourceURL)
	case "playboy":
		return scrapePlayboyGallery(sourceURL)
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

// scrapeMetArtGallery scrapes full metadata from a MetArt gallery page
func scrapeMetArtGallery(url string) (*GalleryMetadata, error) {
	client := GetHTTPClient(url)
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch MetArt gallery: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("MetArt gallery returned status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MetArt gallery: %w", err)
	}

	metadata := &GalleryMetadata{
		Provider:  "MetArt",
		SourceURL: url,
	}

	// Extract description - placeholder selectors
	description := strings.TrimSpace(doc.Find(".description, .synopsis, .gallery-description, p.description").First().Text())
	metadata.Description = description

	// Extract rating - placeholder
	ratingText := strings.TrimSpace(doc.Find(".rating, .score, [itemprop='ratingValue']").First().Text())
	if ratingText != "" {
		fmt.Sscanf(ratingText, "%f", &metadata.Rating)
	}

	// Extract release date - placeholder
	dateText := strings.TrimSpace(doc.Find(".date, time, .release-date, [itemprop='datePublished']").First().Text())
	if dateText != "" {
		// Try common date formats
		for _, layout := range []string{"2006-01-02", "January 2, 2006", "Jan 2, 2006", "02/01/2006"} {
			if parsed, err := time.Parse(layout, dateText); err == nil {
				metadata.ReleaseDate = parsed
				break
			}
		}
	}

	logger.Infof("Scraped MetArt gallery: %s", metadata.Description[:min(50, len(metadata.Description))])
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
