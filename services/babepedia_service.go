package services

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

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
	// Babepedia uses URL format: /babe/Name_Name
	// Convert "Jane Doe" to "Jane_Doe"
	urlName := strings.ReplaceAll(name, " ", "_")

	// Try direct profile access
	profileURL := fmt.Sprintf("%s/babe/%s", s.BaseURL, urlName)

	resp, err := s.Client.Get(profileURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		// Profile exists
		return []IdentifierResult{
			{
				ExternalID:     urlName,
				Name:           name,
				Disambiguation: "Babepedia Profile",
				PreviewData: map[string]interface{}{
					"profile_url": profileURL,
				},
			},
		}, nil
	}

	// Profile not found
	return []IdentifierResult{}, nil
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
