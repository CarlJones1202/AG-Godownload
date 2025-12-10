package services

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
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

// makeRequest creates a request with proper browser headers to avoid 403
func (s *BabepediaService) makeRequest(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Add browser headers to avoid bot detection
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
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

	resp, err := s.makeRequest(profileURL)
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
	resp, err = s.makeRequest(searchURL)
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

// parseProfileData extracts performer data from Babepedia profile HTML
func (s *BabepediaService) parseProfileData(htmlContent string, performerID string) *PersonData {
	data := &PersonData{
		Name:    strings.ReplaceAll(performerID, "_", " "),
		Aliases: []string{},
		Photos:  []string{},
		RawData: make(map[string]interface{}),
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		// Fallback to basic name if parsing fails
		return data
	}

	// Extract name from title or h1
	if h1 := doc.Find("h1").First(); h1.Length() > 0 {
		data.Name = strings.TrimSpace(h1.Text())
	}

	// Babepedia uses text nodes with labels like "Born:", "Birthplace:", etc.
	// We need to find these labels and extract the following text
	pageText := doc.Text()
	fmt.Printf("DEBUG: Full Page Text Length: %d\n", len(pageText))
	// fmt.Printf("DEBUG: Page Text Sample: %q\n", pageText[:1000]) // Inspect start

	// Extract details using text analysis
	if born := extractFieldFromText(pageText, "Born:"); born != "" {
		data.Birthdate = born
		fmt.Printf("DEBUG: Extracted Born: %q\n", born)
	}

	// Birthplace
	if birthplace := extractFieldFromText(pageText, "Birthplace:"); birthplace != "" {
		data.Country = birthplace
		data.RawData["birthplace"] = birthplace
	}

	// Ethnicity
	if ethnicity := extractFieldFromText(pageText, "Ethnicity:"); ethnicity != "" {
		data.Ethnicity = ethnicity
	}

	// Hair Color
	if hair := extractFieldFromText(pageText, "Hair color:"); hair != "" {
		data.HairColor = hair
	}

	// Eye Color
	if eyes := extractFieldFromText(pageText, "Eye color:"); eyes != "" {
		data.EyeColor = eyes
	}

	// Height
	if height := extractFieldFromText(pageText, "Height:"); height != "" {
		data.Height = height
	}

	// Measurements
	if measurements := extractFieldFromText(pageText, "Measurements:"); measurements != "" {
		data.Measurements = measurements
	}

	// Tattoos
	if tattoos := extractFieldFromText(pageText, "Tattoos:"); tattoos != "" {
		data.Tattoos = tattoos
	}

	// Piercings
	if piercings := extractFieldFromText(pageText, "Piercings:"); piercings != "" {
		data.Piercings = piercings
	}

	// Extract aliases
	aliasRegex := regexp.MustCompile(`(?i)(?:aka|also\s+known\s+as|aliases?):\s*([^\n]+)`)
	if match := aliasRegex.FindStringSubmatch(pageText); len(match) > 1 {
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

	// Extract profile images using goquery
	// Start with a permissive approach and filter out obvious non-content images
	seen := make(map[string]bool)
	imageCount := 0
	doc.Find("img").Each(func(i int, sel *goquery.Selection) {
		imageCount++
		src, exists := sel.Attr("src")
		if !exists || src == "" {
			fmt.Printf("DEBUG: Image %d - no src attribute\n", i)
			return
		}

		fmt.Printf("DEBUG: Image %d - src: %s\n", i, src)

		// Skip only obvious UI elements
		srcLower := strings.ToLower(src)
		if strings.Contains(srcLower, "logo") ||
			strings.Contains(srcLower, "icon") ||
			strings.Contains(srcLower, "sprite") ||
			strings.HasSuffix(srcLower, ".gif") {
			fmt.Printf("DEBUG: Image %d - skipped (UI element: logo/icon/sprite/gif)\n", i)
			return
		}

		// Skip very small images (likely icons/buttons)
		if width, exists := sel.Attr("width"); exists {
			if w, err := strconv.Atoi(width); err == nil && w < 50 {
				fmt.Printf("DEBUG: Image %d - skipped (width %d < 50)\n", i, w)
				return
			}
		}
		if height, exists := sel.Attr("height"); exists {
			if h, err := strconv.Atoi(height); err == nil && h < 50 {
				fmt.Printf("DEBUG: Image %d - skipped (height %d < 50)\n", i, h)
				return
			}
		}

		// Accept any image that looks like a real photo
		// Skip only if it's clearly in static/assets folders
		if strings.Contains(src, "/static/") || strings.Contains(src, "/assets/") {
			fmt.Printf("DEBUG: Image %d - skipped (in /static/ or /assets/)\n", i)
			return
		}

		// Make sure it's a full URL
		fullURL := src
		if strings.HasPrefix(src, "http") {
			fullURL = src
		} else if strings.HasPrefix(src, "/") {
			fullURL = s.BaseURL + src
		} else if strings.HasPrefix(src, "//") {
			fullURL = "https:" + src
		} else {
			fmt.Printf("DEBUG: Image %d - skipped (relative URL without leading slash)\n", i)
			return
		}

		// Avoid duplicates
		if seen[fullURL] {
			fmt.Printf("DEBUG: Image %d - skipped (duplicate)\n", i)
			return
		}
		seen[fullURL] = true

		fmt.Printf("DEBUG: Image %d - ACCEPTED: %s\n", i, fullURL)
		data.Photos = append(data.Photos, fullURL)
	})

	fmt.Printf("DEBUG: Total images found: %d, Accepted: %d\n", imageCount, len(data.Photos))

	data.RawData["source"] = "babepedia"
	data.RawData["profile_url"] = fmt.Sprintf("%s/babe/%s", s.BaseURL, performerID)

	return data
}

// extractFieldFromText extracts a field value from plain text after a label
func extractFieldFromText(text, label string) string {
	// Find the label in the text
	idx := strings.Index(text, label)
	if idx == -1 {
		return ""
	}

	// Get text after the label
	after := text[idx+len(label):]

	// Trim leading whitespace, colons, and newlines
	after = strings.TrimLeft(after, " :\t\n\r")

	// List of field labels that indicate the start of a new field
	// These are ordered roughly as they appear on the page
	nextFieldPatterns := []string{
		"Born:",
		"Birthplace:",
		"Nationality:",
		"Ethnicity:",
		"Profession:",
		"Professions:",
		"Sexuality:",
		"Body", // Section header
		"Hair color:",
		"Eye color:",
		"Height:",
		"Weight:",
		"Body type:",
		"Measurements:",
		"Bra/cup size:",
		"Boobs:",
		"Pubic hair:",
		"Tattoos:",
		"Piercings:",
		"Performances", // Section header
		"Solo:",
		"Boy/girl:",
		"Interracial:",
		"Extra", // Section header
		"Instagram",
		"Twitter",
	}

	// Find the nearest next field label
	endIdx := -1
	for _, pattern := range nextFieldPatterns {
		if pos := strings.Index(after, pattern); pos != -1 && pos > 0 {
			if endIdx == -1 || pos < endIdx {
				endIdx = pos
			}
		}
	}

	// If no next field found, look for double newline (paragraph break)
	if endIdx == -1 {
		if nlIdx := strings.Index(after, "\n\n"); nlIdx != -1 {
			endIdx = nlIdx
		}
	}

	// If still no boundary, look for single newline
	if endIdx == -1 {
		if nlIdx := strings.IndexAny(after, "\n\r"); nlIdx != -1 {
			endIdx = nlIdx
		}
	}

	// If still nothing, limit to reasonable length
	if endIdx == -1 {
		endIdx = len(after)
		if endIdx > 200 {
			endIdx = 200
		}
	}

	// Extract the value
	value := after[:endIdx]

	// Normalize whitespace: replace all sequences of whitespace (spaces, tabs, newlines) with a single space
	value = regexp.MustCompile(`\s+`).ReplaceAllString(value, " ")

	// Trim leading/trailing whitespace
	value = strings.TrimSpace(value)

	// Remove common suffixes
	value = strings.TrimSuffix(value, "show conversions")
	value = strings.TrimPrefix(value, ":")
	value = strings.TrimSpace(value)

	// Clean up any remaining artifacts like multiple spaces or trailing punctuation
	value = strings.TrimRight(value, " :\t")

	return value
}
