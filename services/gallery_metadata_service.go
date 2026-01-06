package services

import (
	"bytes"
	"fmt"
	"gallery_api/logger"
	"io"
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

	// Build search query from gallery name and people
	searchQuery := buildSearchQuery(galleryName, people)
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

// searchMetArt searches MetArt for matching galleries
func searchMetArt(query string) ([]GallerySearchResult, error) {
	searchURL := fmt.Sprintf("https://www.metart.com/search/%s", strings.ReplaceAll(query, " ", "+"))

	logger.Infof("[MetArt] Searching with URL: %s", searchURL)

	client := GetHTTPClient(searchURL)
	resp, err := client.Get(searchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search MetArt: %w", err)
	}
	defer resp.Body.Close()

	logger.Infof("[MetArt] Response status: %d", resp.StatusCode)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("MetArt search returned status %d", resp.StatusCode)
	}

	// Read the entire response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read MetArt response: %w", err)
	}

	// Check for explicit block messages in the TITLE or specific error divs
	// Note: We used to check for "ageVerificationBlockedVPNStates" but that is present in the
	// config JSON of every page, so it caused false positives.
	if strings.Contains(strings.ToLower(string(bodyBytes)), "<title>access denied</title>") ||
		strings.Contains(strings.ToLower(string(bodyBytes)), "please verify your age") {
		logger.Errorf("[MetArt] Block detected based on page content.")
		return nil, fmt.Errorf("MetArt blocked access (content check)")
	}

	// Save HTML to file for debugging
	debugFile := "debug_metart_search.html"
	if err := os.WriteFile(debugFile, bodyBytes, 0644); err != nil {
		logger.Warnf("[MetArt] Failed to write debug file: %v", err)
	} else {
		logger.Infof("[MetArt] Saved response to %s (%d bytes)", debugFile, len(bodyBytes))
	}

	// Parse HTML from bytes
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse MetArt search results: %w", err)
	}

	var results []GallerySearchResult

	// Log what we're looking for
	logger.Debugf("[MetArt] Looking for selectors: .gallery-item, .search-result-item, article")

	// Parse search results - this is a placeholder selector that will need refinement
	doc.Find(".gallery-item, .search-result-item, article").Each(func(i int, s *goquery.Selection) {
		if i >= 10 { // Limit to top 10 results
			return
		}

		logger.Debugf("[MetArt] Processing result %d", i+1)

		title := strings.TrimSpace(s.Find("h2, h3, .title, .gallery-title").First().Text())
		url, _ := s.Find("a").First().Attr("href")
		thumbnail, _ := s.Find("img").First().Attr("src")
		releaseDate := strings.TrimSpace(s.Find(".date, time, .release-date").First().Text())

		logger.Debugf("[MetArt] Result %d: title=%q, url=%q, thumb=%q", i+1, title, url, thumbnail)

		// Make URL absolute if relative
		if url != "" && !strings.HasPrefix(url, "http") {
			url = "https://www.metart.com" + url
		}

		// Make thumbnail URL absolute if relative
		if thumbnail != "" && !strings.HasPrefix(thumbnail, "http") {
			thumbnail = "https://www.metart.com" + thumbnail
		}

		if title != "" && url != "" {
			results = append(results, GallerySearchResult{
				Provider:    "MetArt",
				Title:       title,
				URL:         url,
				Thumbnail:   thumbnail,
				ReleaseDate: releaseDate,
			})
			logger.Infof("[MetArt] Added result: %s", title)
		}
	})

	logger.Debugf("Found %d MetArt results", len(results))
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
