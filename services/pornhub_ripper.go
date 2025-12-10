package services

import (
	"encoding/json"
	"fmt"
	"gallery_api/logger"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// PornhubMediaDefinition represents a video source in the flashvars
type PornhubMediaDefinition struct {
	DefaultQuality bool            `json:"defaultQuality"`
	Format         string          `json:"format"`
	Quality        json.RawMessage `json:"quality"` // Can be string ("1080") or int
	VideoUrl       string          `json:"videoUrl"`
}

// RipPornhub extracts the highest quality video URL from a Pornhub page
func RipPornhub(url string) (string, string, error) {
	logger.Infof("Attempting to rip Pornhub video from: %s", url)

	// 1. Fetch the page
	// We need a proper User-Agent to avoid being blocked or served partial content
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("server returned status: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read response body: %w", err)
	}
	bodyString := string(bodyBytes)

	// 2. Extract Flashvars
	// Pattern: var flashvars_\d+ = {...};
	// We look for the variable assignment	// Try to find the flashvars_{video_id} JavaScript variable
	// Pattern: var flashvars_449554261 = {...};
	flashvarsPattern := regexp.MustCompile(`var\s+flashvars_\d+\s*=\s*(\{.+?\});`)
	matches := flashvarsPattern.FindStringSubmatch(bodyString)

	var flashvarsJSON string
	if len(matches) > 1 {
		flashvarsJSON = matches[1]
		logger.Debugf("Found flashvars JavaScript variable")
	} else {
		// Fallback: try older patterns
		patterns := []string{
			`flashvars\s*=\s*(\{[^;]+\});`,
			`"mediaDefinitions":\s*(\[[^\]]+\])`,
		}

		for _, pattern := range patterns {
			re := regexp.MustCompile(pattern)
			fallbackMatches := re.FindStringSubmatch(bodyString)
			if len(fallbackMatches) > 1 {
				flashvarsJSON = fallbackMatches[1]
				logger.Debugf("Found flashvars with fallback pattern: %s", pattern)
				break
			}
		}
	}

	if flashvarsJSON == "" {
		// DEBUG: Save the failed HTML to a file for inspection
		os.WriteFile("last_failed_pornhub.html", []byte(bodyString), 0644)
		logger.Errorf("Failed to find flashvars. Saved page content to last_failed_pornhub.html")
		return "", "", fmt.Errorf("could not find flashvars in page content")
	}
	jsonContent := flashvarsJSON

	// 3. Parse JSON
	type FlashVars struct {
		MediaDefinitions []PornhubMediaDefinition `json:"mediaDefinitions"`
		VideoTitle       string                   `json:"video_title"`
	}
	var fv FlashVars
	if err := json.Unmarshal([]byte(jsonContent), &fv); err != nil {
		return "", "", fmt.Errorf("failed to parse flashvars JSON: %w", err)
	}

	if len(fv.MediaDefinitions) == 0 {
		return "", "", fmt.Errorf("no media definitions found")
	}

	// 4. Extract Title
	// Prefer title from flashvars, fall back to parsing <title> tag if needed
	title := fv.VideoTitle
	if title == "" {
		titleRegex := regexp.MustCompile(`<title>(.*?)- Pornhub\.com</title>`)
		titleMatches := titleRegex.FindStringSubmatch(bodyString)
		if len(titleMatches) > 1 {
			title = strings.TrimSpace(titleMatches[1])
		} else {
			title = "Unknown Pornhub Video"
		}
	}

	// 5. Find highest quality mp4
	var bestUrl string
	var maxQuality int

	for _, md := range fv.MediaDefinitions {
		if md.Format != "mp4" {
			continue
		}
		if md.VideoUrl == "" {
			continue
		}

		// Parse quality
		// It comes as a RawMessage which we need to convert to int
		qStr := string(md.Quality)
		qStr = strings.Trim(qStr, "\"") // remove quotes if present
		qStr = strings.TrimSuffix(qStr, "p")

		q, _ := strconv.Atoi(qStr)

		if q > maxQuality {
			maxQuality = q
			bestUrl = md.VideoUrl
		}
	}

	if bestUrl == "" {
		return "", "", fmt.Errorf("no suitable MP4 video found in definitions")
	}

	logger.Infof("Found Pornhub video: %s (Quality: %dp)", title, maxQuality)

	// Sometimes the URL needs unescaping if it contains \/
	bestUrl = strings.ReplaceAll(bestUrl, `\/`, `/`)

	return bestUrl, title, nil
}
