package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"gallery_api/logger"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// RipTnaFlix extracts the direct video URL from a TnaFlix page
func RipTnaFlix(pageURL string) (string, error) {
	logger.Debugf("Starting RipTnaFlix for %s", pageURL)

	// Extract video ID from URL
	// URL format: https://www.tnaflix.com/amateur-porn/nastya-vs-the-world/video6504877
	videoIDRegex := regexp.MustCompile(`video(\d+)`)
	matches := videoIDRegex.FindStringSubmatch(pageURL)
	if len(matches) < 2 {
		return "", fmt.Errorf("could not extract video ID from URL: %s", pageURL)
	}
	videoID := matches[1]
	logger.Debugf("Extracted video ID: %s", videoID)

	// Fetch the page to look for additional video configuration
	client := &http.Client{}
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101 Firefox/91.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("page returned status %d", resp.StatusCode)
	}

	// Parse HTML to look for video configuration
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("parsing HTML: %w", err)
	}

	// Look for video source in various places
	var videoURL string

	// Method 1: Check for HTML5 video source tags
	doc.Find("video source").Each(func(i int, s *goquery.Selection) {
		if src, exists := s.Attr("src"); exists && strings.Contains(src, ".mp4") {
			videoURL = src
			logger.Debugf("Found video source in HTML5 tag: %s", videoURL)
		}
	})

	// Method 2: Look for JavaScript config variables
	if videoURL == "" {
		doc.Find("script").Each(func(i int, s *goquery.Selection) {
			scriptContent := s.Text()

			// Look for common video URL patterns in JavaScript
			urlPatterns := []string{
				`video_url["\s:=]+["']([^"']+\.mp4[^"']*)["']`,
				`file["\s:=]+["']([^"']+\.mp4[^"']*)["']`,
				`src["\s:=]+["']([^"']+\.mp4[^"']*)["']`,
				`https://[^"'\s]+\.mp4`,
			}

			for _, pattern := range urlPatterns {
				re := regexp.MustCompile(pattern)
				if matches := re.FindStringSubmatch(scriptContent); len(matches) > 0 {
					candidate := matches[len(matches)-1]
					if strings.Contains(candidate, ".mp4") {
						videoURL = candidate
						logger.Debugf("Found video URL in JavaScript: %s", videoURL)
						return
					}
				}
			}
		})
	}

	// Method 3: Construct URL using known CDN patterns
	if videoURL == "" {
		// Try multiple quality levels and CDN patterns
		cdnPatterns := []string{
			fmt.Sprintf("https://cdn.tnaflix.com/contents/videos/%s/720p.mp4", videoID),
			fmt.Sprintf("https://cdn.tnaflix.com/contents/videos/%s/480p.mp4", videoID),
			fmt.Sprintf("https://static.tnaflix.com/contents/videos_sources/%s/file.mp4", videoID),
			fmt.Sprintf("https://media.tnaflix.com/progressive/%s/720/video.mp4", videoID),
		}

		for _, pattern := range cdnPatterns {
			// Try HEAD request to check if URL exists
			headReq, _ := http.NewRequest("HEAD", pattern, nil)
			headReq.Header.Set("Referer", pageURL)
			headReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101 Firefox/91.0")

			headResp, err := client.Do(headReq)
			if err == nil && headResp.StatusCode == 200 {
				videoURL = pattern
				logger.Debugf("Found working CDN URL: %s", videoURL)
				headResp.Body.Close()
				break
			}
			if headResp != nil {
				headResp.Body.Close()
			}
		}
	}

	if videoURL == "" {
		return "", fmt.Errorf("could not find video URL for video ID %s", videoID)
	}

	return videoURL, nil
}

// DownloadVideo downloads a video from a direct URL and saves it with hash-based naming
func DownloadVideo(videoURL string, sourceName string, pageURL string) (*DownloadImageResult, error) {
	logger.Infof("Downloading video from %s", videoURL)

	// Create HTTP client with proper headers
	client := &http.Client{}
	req, err := http.NewRequest("GET", videoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Set headers to bypass hotlink protection
	req.Header.Set("Referer", pageURL)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101 Firefox/91.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading video: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("video download returned status %d", resp.StatusCode)
	}

	// Read the entire video into memory for hashing
	// Note: For very large videos, we might want to stream this differently
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading video data: %w", err)
	}

	// Calculate SHA-256 hash
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	// Determine extension (should be .mp4 for most cases)
	ext := ".mp4"
	if strings.HasSuffix(strings.ToLower(videoURL), ".webm") {
		ext = ".webm"
	} else if strings.HasSuffix(strings.ToLower(videoURL), ".mkv") {
		ext = ".mkv"
	}

	filename := hashStr + ext

	// Sanitize source name for directory
	sourceDir := SanitizeDirectoryName(sourceName)
	if sourceDir == "" {
		sourceDir = "unknown"
	}

	// Create source subdirectory
	fullDir := filepath.Join(UploadsDir, sourceDir)
	if err := os.MkdirAll(fullDir, 0755); err != nil {
		return nil, fmt.Errorf("creating directory: %w", err)
	}

	// Full path for the video
	destPath := filepath.Join(fullDir, filename)

	// Check if file already exists
	fileExists := false
	if _, err := os.Stat(destPath); err == nil {
		fileExists = true
		logger.Debugf("Video file already exists: %s", destPath)
	}

	if !fileExists {
		// Write the file
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return nil, fmt.Errorf("writing video file: %w", err)
		}
		logger.Infof("Saved video to: %s", destPath)
	}

	// Generate thumbnail
	if _, err := GenerateVideoThumbnail(destPath); err != nil {
		logger.Warnf("Failed to generate video thumbnail: %v", err)
	}

	// Generate trickplay data
	if err := GenerateTrickplayData(destPath); err != nil {
		logger.Warnf("Failed to generate trickplay data: %v", err)
	}

	return &DownloadImageResult{
		Path:           destPath,
		DominantColors: "[]", // Videos don't need color extraction
	}, nil
}

// IsVideoURL checks if a URL points to a known video hosting site
func IsVideoURL(url string) bool {
	videoSites := []string{
		"tnaflix.com",
		"pornhub.com",
		"xvideos.com",
		"redtube.com",
		"tube8.com",
		"youporn.com",
		"spankbang.com",
	}

	lowerURL := strings.ToLower(url)
	for _, site := range videoSites {
		if strings.Contains(lowerURL, site) {
			return true
		}
	}

	return false
}
