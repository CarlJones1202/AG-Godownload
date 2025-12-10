package services

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

type BabepediaService struct {
	BaseURL string
	Client  *http.Client
}

func NewBabepediaService() *BabepediaService {
	return &BabepediaService{
		BaseURL: "https://www.babepedia.com",
		Client:  &http.Client{},
	}
}

// GetName implements IdentifierProvider interface
func (s *BabepediaService) GetName() string {
	return "babepedia"
}

// Search implements IdentifierProvider interface
func (s *BabepediaService) Search(name string) ([]IdentifierResult, error) {
	var results []IdentifierResult

	// 1. Try direct profile access (Fast Path)
	// Babepedia uses URL format: /babe/Name_Name
	urlName := strings.ReplaceAll(name, " ", "_")
	profileURL := fmt.Sprintf("%s/babe/%s", s.BaseURL, urlName)

	resp, err := s.Client.Get(profileURL)
	if err == nil && resp.StatusCode == 200 {
		resp.Body.Close()
		results = append(results, IdentifierResult{
			ExternalID:     urlName,
			Name:           name,
			Disambiguation: "Direct Match",
			PreviewData: map[string]interface{}{
				"profile_url": profileURL,
			},
		})
		// If we found a direct match, we might still want to search if the user
		// might be looking for someone else, but usually direct match is good enough.
		// However, to be robust, let's also search if the direct match isn't perfect
		// or if we want to show alternatives. For now, returning direct match is efficient.
		return results, nil
	}
	if resp != nil {
		resp.Body.Close()
	}

	// 2. Search Scraping (Fallback)
	searchURL := fmt.Sprintf("%s/search/%s", s.BaseURL, url.QueryEscape(name))
	resp, err = s.Client.Get(searchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("search returned status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parsing search results: %w", err)
	}

	// Babepedia search results usually list profiles.
	// We look for links pointing to /babe/XXXX
	seen := make(map[string]bool)

	// Selector might need adjustment based on actual site structure
	// Assuming a generic list of links for now since I cannot inspect the DOM
	doc.Find("a[href^='/babe/']").Each(func(i int, sel *goquery.Selection) {
		href, _ := sel.Attr("href")
		// href is like /babe/Name_Surname
		externalID := strings.TrimPrefix(href, "/babe/")

		if seen[externalID] || externalID == "" {
			return
		}
		seen[externalID] = true

		displayName := strings.TrimSpace(sel.Text())
		if displayName == "" {
			displayName = strings.ReplaceAll(externalID, "_", " ")
		}

		// Try to find an image thumbnail if present inside the anchor or nearby
		thumbURL := ""
		if img := sel.Find("img").First(); img.Length() > 0 {
			thumbURL = img.AttrOr("src", "")
		}

		results = append(results, IdentifierResult{
			ExternalID:     externalID,
			Name:           displayName,
			Disambiguation: "Search Result",
			PreviewData: map[string]interface{}{
				"profile_url": fmt.Sprintf("%s%s", s.BaseURL, href),
				"thumbnail":   thumbURL,
			},
		})
	})

	return results, nil
}

// GetDetails implements IdentifierProvider interface
func (s *BabepediaService) GetDetails(externalID string) (*PersonData, error) {
	profileURL := fmt.Sprintf("%s/babe/%s", s.BaseURL, externalID)

	resp, err := s.Client.Get(profileURL)
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

// parseProfileData extracts performer data from Babepedia profile HTML
func (s *BabepediaService) parseProfileData(htmlContent string, performerID string) *PersonData {
	data := &PersonData{
		Name:    strings.ReplaceAll(performerID, "_", " "),
		Aliases: []string{},
		Photos:  []string{},
		RawData: make(map[string]interface{}),
	}

	// Extract name from title or h1
	nameRegex := regexp.MustCompile(`<h1[^>]*>([^<]+)</h1>`)
	if match := nameRegex.FindStringSubmatch(htmlContent); len(match) > 1 {
		data.Name = strings.TrimSpace(match[1])
	}

	// Babepedia has structured data in info boxes
	// Extract birthdate
	if birthdate := extractBabepediaField(htmlContent, `(?i)born[:\s]*</[^>]+>\s*([^<]+)`); birthdate != "" {
		data.Birthdate = birthdate
	}

	// Birthplace
	if birthplace := extractBabepediaField(htmlContent, `(?i)birthplace[:\s]*</[^>]+>\s*([^<]+)`); birthplace != "" {
		data.Country = birthplace
		data.RawData["birthplace"] = birthplace
	}

	// Ethnicity
	if ethnicity := extractBabepediaField(htmlContent, `(?i)ethnicity[:\s]*</[^>]+>\s*([^<]+)`); ethnicity != "" {
		data.Ethnicity = ethnicity
	}

	// Hair Color
	if hair := extractBabepediaField(htmlContent, `(?i)hair\s+color[:\s]*</[^>]+>\s*([^<]+)`); hair != "" {
		data.HairColor = hair
	}

	// Eye Color
	if eyes := extractBabepediaField(htmlContent, `(?i)eye\s+color[:\s]*</[^>]+>\s*([^<]+)`); eyes != "" {
		data.EyeColor = eyes
	}

	// Height
	if height := extractBabepediaField(htmlContent, `(?i)height[:\s]*</[^>]+>\s*([^<]+)`); height != "" {
		data.Height = height
	}

	// Measurements
	if measurements := extractBabepediaField(htmlContent, `(?i)measurements[:\s]*</[^>]+>\s*([^<]+)`); measurements != "" {
		data.Measurements = measurements
	}

	// Tattoos
	if tattoos := extractBabepediaField(htmlContent, `(?i)tattoos[:\s]*</[^>]+>\s*([^<]+)`); tattoos != "" {
		data.Tattoos = tattoos
	}

	// Piercings
	if piercings := extractBabepediaField(htmlContent, `(?i)piercings[:\s]*</[^>]+>\s*([^<]+)`); piercings != "" {
		data.Piercings = piercings
	}

	// Extract aliases
	aliasRegex := regexp.MustCompile(`(?i)(?:aka|also\s+known\s+as|aliases?)[:\s]*</[^>]+>\s*([^<]+)`)
	if match := aliasRegex.FindStringSubmatch(htmlContent); len(match) > 1 {
		aliasText := match[1]
		aliases := regexp.MustCompile(`[,;]`).Split(aliasText, -1)
		for _, alias := range aliases {
			alias = strings.TrimSpace(alias)
			if alias != "" && alias != data.Name {
				data.Aliases = append(data.Aliases, alias)
			}
		}
	}

	// Extract social media
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
			if !strings.Contains(imgURL, "icon") && !strings.Contains(imgURL, "logo") && !strings.Contains(imgURL, "avatar") {
				data.Photos = append(data.Photos, imgURL)
				if len(data.Photos) >= 10 {
					break
				}
			}
		}
	}

	data.RawData["source"] = "babepedia"
	data.RawData["profile_url"] = fmt.Sprintf("%s/babe/%s", s.BaseURL, performerID)

	return data
}

// extractBabepediaField is a helper to extract field values from Babepedia HTML
func extractBabepediaField(htmlContent, pattern string) string {
	re := regexp.MustCompile(pattern)
	if match := re.FindStringSubmatch(htmlContent); len(match) > 1 {
		value := strings.TrimSpace(match[1])
		value = html.UnescapeString(value)
		value = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(value, "")
		return strings.TrimSpace(value)
	}
	return ""
}
