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
	"github.com/kkdai/youtube/v2"
)

// RipYouTube downloads a YouTube video and returns the file path and title
func RipYouTube(pageURL string) (string, string, error) {
	logger.Infof("Starting RipYouTube for %s", pageURL)

	// Create YouTube client
	client := youtube.Client{}

	// Get video info
	video, err := client.GetVideo(pageURL)
	if err != nil {
		return "", "", fmt.Errorf("failed to get YouTube video info: %w", err)
	}

	logger.Infof("YouTube video title: %s", video.Title)

	// Get formats with audio
	formats := video.Formats.WithAudioChannels()
	if len(formats) == 0 {
		return "", "", fmt.Errorf("no formats with audio found for video")
	}

	// Select highest quality format
	// Formats are typically ordered, but let's find the one with highest quality
	var bestFormat *youtube.Format
	var maxQuality int

	for i := range formats {
		format := &formats[i]
		// Prefer formats with higher quality (resolution)
		quality := format.Width * format.Height
		if quality > maxQuality {
			maxQuality = quality
			bestFormat = format
		}
	}

	if bestFormat == nil {
		// Fallback to first format
		bestFormat = &formats[0]
	}

	logger.Infof("Selected format: %s (Quality: %dx%d, Bitrate: %d)",
		bestFormat.MimeType, bestFormat.Width, bestFormat.Height, bestFormat.Bitrate)

	// Get stream
	stream, _, err := client.GetStream(video, bestFormat)
	if err != nil {
		return "", "", fmt.Errorf("failed to get video stream: %w", err)
	}
	defer stream.Close()

	// Create temp file for download
	tempFile, err := os.CreateTemp("", "youtube_*.mp4")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath) // Clean up temp file

	// Download to temp file
	logger.Infof("Downloading YouTube video to temp file...")
	_, err = io.Copy(tempFile, stream)
	tempFile.Close()
	if err != nil {
		return "", "", fmt.Errorf("failed to download video: %w", err)
	}

	// Calculate hash of downloaded file
	f, err := os.Open(tempPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to open temp file for hashing: %w", err)
	}
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", "", fmt.Errorf("failed to hash video: %w", err)
	}
	hashStr := hex.EncodeToString(hash.Sum(nil))
	f.Close()

	// Determine final filename and path
	filename := hashStr + ".mp4"

	// Return the temp path and title
	// The caller will handle moving to final location
	return tempPath, video.Title, nil
}

// RipTnaFlix extracts the direct video URL and title from a TnaFlix page
func RipTnaFlix(pageURL string) (string, string, error) {
	logger.Debugf("Starting RipTnaFlix for %s", pageURL)

	// Extract video ID from URL
	// URL format: https://www.tnaflix.com/amateur-porn/nastya-vs-the-world/video6504877
	videoIDRegex := regexp.MustCompile(`video(\d+)`)
	matches := videoIDRegex.FindStringSubmatch(pageURL)
	if len(matches) < 2 {
		return "", "", fmt.Errorf("could not extract video ID from URL: %s", pageURL)
	}
	videoID := matches[1]
	logger.Debugf("Extracted video ID: %s", videoID)

	// Fetch the page to look for additional video configuration
	client := &http.Client{}
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101 Firefox/91.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("fetching page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("page returned status %d", resp.StatusCode)
	}

	// Parse HTML to look for video configuration
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("parsing HTML: %w", err)
	}

	// Extract Title
	title := strings.TrimSpace(doc.Find("h1").First().Text())
	// Use regex to remove trailing site name if common, e.g., "- TnaFlix"
	title = strings.TrimSuffix(title, " - TnaFlix")
	if title == "" {
		title = "Unknown Video " + videoID
	}

	// Look for video source in various places
	var videoURL string

	// Method 1: Check for HTML5 video source tags (Highest Priority)
	type VideoCandidate struct {
		URL     string
		Quality int
	}
	var candidates []VideoCandidate

	doc.Find("video source").Each(func(i int, s *goquery.Selection) {
		src, exists := s.Attr("src")
		if !exists || !strings.Contains(src, ".mp4") {
			return
		}

		quality := 0

		// Try to get size attribute
		if sizeStr, ok := s.Attr("size"); ok {
			var q int
			fmt.Sscanf(sizeStr, "%d", &q)
			quality = q
		}

		// Fallback: try to parse from URL (e.g. "1080p")
		if quality == 0 {
			re := regexp.MustCompile(`(\d{3,4})p`)
			matches := re.FindStringSubmatch(src)
			if len(matches) > 1 {
				var q int
				fmt.Sscanf(matches[1], "%d", &q)
				quality = q
			}
		}

		logger.Debugf("Found candidate: %s (Quality: %d)", src, quality)
		candidates = append(candidates, VideoCandidate{URL: src, Quality: quality})
	})

	// Select best quality
	if len(candidates) > 0 {
		bestCandidate := candidates[0]
		for _, c := range candidates {
			if c.Quality > bestCandidate.Quality {
				bestCandidate = c
			}
		}
		videoURL = bestCandidate.URL
		logger.Infof("Selected best quality video (%dp): %s", bestCandidate.Quality, videoURL)
	}

	// Method 2: Look for JavaScript config variables (Fallback)
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

	// Method 3: Check for HTML5 video source tags (Lowest Priority usually defaults)
	if videoURL == "" {
		doc.Find("video source").Each(func(i int, s *goquery.Selection) {
			if src, exists := s.Attr("src"); exists && strings.Contains(src, ".mp4") {
				videoURL = src
				logger.Debugf("Found video source in HTML5 tag: %s", videoURL)
			}
		})
	}

	// Fallback Check
	if videoURL == "" {
		// Fallback for base "file.mp4" which is usually low quality but better than nothing
		fallbackURL := fmt.Sprintf("https://static.tnaflix.com/contents/videos_sources/%s/file.mp4", videoID)
		headReq, _ := http.NewRequest("HEAD", fallbackURL, nil)
		headReq.Header.Set("Referer", pageURL)
		headResp, err := client.Do(headReq)
		if err == nil && headResp.StatusCode == 200 {
			videoURL = fallbackURL
			logger.Debugf("Found fallback CDN URL: %s", videoURL)
			headResp.Body.Close()
		} else if headResp != nil {
			headResp.Body.Close()
		}
	}

	if videoURL == "" {
		return "", "", fmt.Errorf("could not find video URL for video ID %s", videoID)
	}

	return videoURL, title, nil
}

// DownloadVideo downloads a video from a direct URL and saves it with hash-based naming
func DownloadVideo(videoURL string, sourceName string, pageURL string, title string) (*DownloadImageResult, error) {
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

	// Parse Video Metadata
	meta, err := GetVideoMetadata(destPath)
	if err != nil {
		logger.Warnf("Failed to get video metadata: %v", err)
		meta = &VideoMetadata{}
	}

	return &DownloadImageResult{
		Path:           destPath,
		Title:          title,
		Duration:       meta.Duration,
		Width:          meta.Width,
		Height:         meta.Height,
		SizeMB:         meta.SizeMB,
		DominantColors: "[]", // Videos don't need color extraction
	}, nil
}

// RipPMVHaven extracts the video URL from a PMVHaven page
func RipPMVHaven(pageURL string) (string, string, error) {
	logger.Debugf("Starting RipPMVHaven for %s", pageURL)

	client := &http.Client{}
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101 Firefox/91.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("fetching page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("page returned status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("parsing HTML: %w", err)
	}

	title := strings.TrimSpace(doc.Find("h1").First().Text())
	title = strings.TrimSuffix(title, " - PMVHaven")
	if title == "" {
		title = "Unknown Video"
	}

	var videoURL string

	// Method 1: Look for regex matches in all script tags
	// We look for patterns like:
	// video_url: "..."
	// contentUrl: "..." (but avoid the page URL itself)
	// .mp4 inside any string
	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		if videoURL != "" {
			return
		}
		text := s.Text()

		// Regex to find mp4 URLs
		re := regexp.MustCompile(`https?://[^"'\s<>]+\.mp4`)
		matches := re.FindAllString(text, -1)

		if len(matches) > 0 {
			logger.Debugf("Script %d: Found %d potential mp4 matches", i, len(matches))
		}

		for _, match := range matches {
			logger.Debugf("Checking match: %s", match)
			// Filter out obviously wrong ones if needed
			if strings.Contains(match, "pmvhaven.com") {
				videoURL = match
				logger.Infof("Found candidate video URL in script: %s", videoURL)
				return
			}
		}
	})

	// Method 2: Check standard meta tags (fallback)
	if videoURL == "" {
		videoURL, _ = doc.Find("meta[property='og:video']").Attr("content")
		if videoURL == "" {
			videoURL, _ = doc.Find("meta[property='og:video:url']").Attr("content")
		}
		// PMVHaven might use og:video for the page URL, so verify it ends in .mp4
		if videoURL != "" && !strings.HasSuffix(videoURL, ".mp4") {
			logger.Debugf("Ignoring non-mp4 og:video: %s", videoURL)
			videoURL = ""
		}
	}

	// Method 3: Check for video source tags
	if videoURL == "" {
		doc.Find("video source").Each(func(i int, s *goquery.Selection) {
			if src, exists := s.Attr("src"); exists && strings.Contains(src, ".mp4") {
				videoURL = src
				logger.Debugf("Found video source in HTML5 tag: %s", videoURL)
			}
		})
	}

	if videoURL == "" {
		return "", "", fmt.Errorf("could not find video URL on %s", pageURL)
	}

	return videoURL, title, nil
}
