package services

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

type FreeOnesService struct {
	BaseURL string
	Client  *http.Client
}

func NewFreeOnesService() *FreeOnesService {
	return &FreeOnesService{
		BaseURL: "https://www.freeones.com",
		Client:  &http.Client{},
	}
}

// makeRequest creates a request with proper browser headers to avoid bot detection
func (s *FreeOnesService) makeRequest(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Add browser headers to avoid bot detection
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Cache-Control", "max-age=0")

	return s.Client.Do(req)
}

// GetName implements IdentifierProvider interface
func (s *FreeOnesService) GetName() string {
	return "freeones"
}

// Search implements IdentifierProvider interface
func (s *FreeOnesService) Search(name string) ([]IdentifierResult, error) {
	searchURL := fmt.Sprintf("%s/search/?q=%s&t=1", s.BaseURL, url.QueryEscape(name))

	resp, err := s.makeRequest(searchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse search results
	results := s.parseSearchResults(string(body))

	return results, nil
}

// GetDetails implements IdentifierProvider interface
func (s *FreeOnesService) GetDetails(externalID string) (*PersonData, error) {
	profileURL := fmt.Sprintf("%s/%s/profile", s.BaseURL, externalID)

	resp, err := s.makeRequest(profileURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch profile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("profile not found: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse profile data
	data := s.parseProfileData(string(body), externalID)

	return data, nil
}

// parseSearchResults extracts search results from HTML
func (s *FreeOnesService) parseSearchResults(htmlContent string) []IdentifierResult {
	var results []IdentifierResult

	// Look for performer links in search results
	// FreeOnes uses links like: /performer-name/profile or /performer-name/bio
	linkRegex := regexp.MustCompile(`href="/([\w-]+)/(profile|bio)"[^>]*>([^<]+)</a>`)
	matches := linkRegex.FindAllStringSubmatch(htmlContent, -1)

	seen := make(map[string]bool)
	for _, match := range matches {
		if len(match) >= 4 {
			performerID := match[1]
			performerName := strings.TrimSpace(match[3])

			// Avoid duplicates
			if seen[performerID] {
				continue
			}
			seen[performerID] = true

			results = append(results, IdentifierResult{
				ExternalID:     performerID,
				Name:           performerName,
				Disambiguation: "",
				PreviewData: map[string]interface{}{
					"profile_url": fmt.Sprintf("%s/%s/profile", s.BaseURL, performerID),
				},
			})
		}
	}

	return results
}

// parseProfileData extracts performer data from profile HTML
func (s *FreeOnesService) parseProfileData(htmlContent string, performerID string) *PersonData {
	data := &PersonData{
		Name:    performerID,
		Aliases: []string{},
		Photos:  []string{},
		RawData: make(map[string]interface{}),
	}

	// Extract performer name from title or h1
	nameRegex := regexp.MustCompile(`<h1[^>]*>([^<]+)</h1>`)
	if match := nameRegex.FindStringSubmatch(htmlContent); len(match) > 1 {
		data.Name = strings.TrimSpace(match[1])
	}

	// Extract bio/stats - FreeOnes typically has a stats section
	// Look for common patterns like "Born:", "Birthplace:", etc.

	// Birthdate
	if birthdate := extractField(htmlContent, `(?i)born[:\s]*</[^>]+>\s*([^<]+)`); birthdate != "" {
		data.Birthdate = birthdate
	}

	// Birthplace/Country
	if birthplace := extractField(htmlContent, `(?i)birthplace[:\s]*</[^>]+>\s*([^<]+)`); birthplace != "" {
		data.Country = birthplace
		data.RawData["birthplace"] = birthplace
	}

	// Ethnicity
	if ethnicity := extractField(htmlContent, `(?i)ethnicity[:\s]*</[^>]+>\s*([^<]+)`); ethnicity != "" {
		data.Ethnicity = ethnicity
	}

	// Hair Color
	if hair := extractField(htmlContent, `(?i)hair\s+color[:\s]*</[^>]+>\s*([^<]+)`); hair != "" {
		data.HairColor = hair
	}

	// Eye Color
	if eyes := extractField(htmlContent, `(?i)eye\s+color[:\s]*</[^>]+>\s*([^<]+)`); eyes != "" {
		data.EyeColor = eyes
	}

	// Height
	if height := extractField(htmlContent, `(?i)height[:\s]*</[^>]+>\s*([^<]+)`); height != "" {
		data.Height = height
	}

	// Measurements
	if measurements := extractField(htmlContent, `(?i)measurements[:\s]*</[^>]+>\s*([^<]+)`); measurements != "" {
		data.Measurements = measurements
	}

	// Tattoos
	if tattoos := extractField(htmlContent, `(?i)tattoos[:\s]*</[^>]+>\s*([^<]+)`); tattoos != "" {
		data.Tattoos = tattoos
	}

	// Piercings
	if piercings := extractField(htmlContent, `(?i)piercings[:\s]*</[^>]+>\s*([^<]+)`); piercings != "" {
		data.Piercings = piercings
	}

	// Career start/end
	if careerStart := extractField(htmlContent, `(?i)career\s+start[:\s]*</[^>]+>\s*([^<]+)`); careerStart != "" {
		data.CareerLength = careerStart
		data.RawData["career_start"] = careerStart
	}

	// Extract aliases
	aliasRegex := regexp.MustCompile(`(?i)(?:aka|also\s+known\s+as|aliases?)[:\s]*</[^>]+>\s*([^<]+)`)
	if match := aliasRegex.FindStringSubmatch(htmlContent); len(match) > 1 {
		aliasText := match[1]
		// Split by common delimiters
		aliases := regexp.MustCompile(`[,;]`).Split(aliasText, -1)
		for _, alias := range aliases {
			alias = strings.TrimSpace(alias)
			if alias != "" && alias != data.Name {
				data.Aliases = append(data.Aliases, alias)
			}
		}
	}

	// Extract social media links
	twitterRegex := regexp.MustCompile(`(?i)(?:twitter\.com|x\.com)/([a-zA-Z0-9_]+)`)
	if match := twitterRegex.FindStringSubmatch(htmlContent); len(match) > 1 {
		data.Twitter = match[1]
	}

	instagramRegex := regexp.MustCompile(`(?i)instagram\.com/([a-zA-Z0-9_.]+)`)
	if match := instagramRegex.FindStringSubmatch(htmlContent); len(match) > 1 {
		data.Instagram = match[1]
	}

	// Extract profile images
	imgRegex := regexp.MustCompile(`<img[^>]+src="(https?://[^"]+(?:jpg|jpeg|png|webp))"[^>]*>`)
	matches := imgRegex.FindAllStringSubmatch(htmlContent, -1)
	for _, match := range matches {
		if len(match) > 1 {
			imgURL := match[1]
			// Filter out small icons and logos
			if !strings.Contains(imgURL, "icon") && !strings.Contains(imgURL, "logo") {
				data.Photos = append(data.Photos, imgURL)
				if len(data.Photos) >= 10 {
					break // Limit to 10 photos
				}
			}
		}
	}

	data.RawData["source"] = "freeones"
	data.RawData["profile_url"] = fmt.Sprintf("%s/%s/profile", s.BaseURL, performerID)

	return data
}

// extractField is a helper to extract a field value from HTML using regex
func extractField(htmlContent, pattern string) string {
	re := regexp.MustCompile(pattern)
	if match := re.FindStringSubmatch(htmlContent); len(match) > 1 {
		// Clean up HTML entities and extra whitespace
		value := strings.TrimSpace(match[1])
		value = html.UnescapeString(value)
		// Remove any remaining HTML tags
		value = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(value, "")
		return strings.TrimSpace(value)
	}
	return ""
}
