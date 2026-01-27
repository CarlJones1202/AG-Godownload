package services

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"gallery_api/logger"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/kkdai/youtube/v2"
)

// RipYouTube downloads a YouTube video and returns the file path and title
func RipYouTube(pageURL string) (string, string, error) {
	logger.Infof("Starting RipYouTube for %s", pageURL)

	// Create YouTube client
	client := youtube.Client{}

	// Check for cookies to bypass age restrictions
	cookieFile := "youtube_cookies.txt"
	if _, err := os.Stat(cookieFile); err == nil {
		jar, _ := cookiejar.New(nil)
		if err := LoadCookies(jar, cookieFile); err == nil {
			client.HTTPClient = &http.Client{
				Jar: jar,
			}
			logger.Infof("Loaded YouTube cookies from %s", cookieFile)
		} else {
			logger.Warnf("Failed to load YouTube cookies from %s: %v", cookieFile, err)
		}
	} else {
		// Log where we are looking for cookies to help the user
		absPath, _ := filepath.Abs(cookieFile)
		logger.Warnf("YouTube cookies file not found at %s. Age-restricted videos may fail.", absPath)
	}

	// Get video info
	video, err := client.GetVideo(pageURL)
	if err != nil {
		// If initial fetch fails, try it with a different client as well
		// Sometimes TVClient works better for age-restricted content
		// This is a common workaround in youtube-dl based tools
		logger.Warnf("Primary YouTube fetch failed: %v. Retrying with different client...", err)

		// Note: ClientType is not directly switchable on the Client struct easily in v2
		// without creating a new client or using internal methods.
		// However, we can try to re-init some settings if needed.
		// For now, let's just log and fail if the above didn't work.
		return "", "", fmt.Errorf("failed to get YouTube video info: %w", err)
	}

	logger.Infof("YouTube video title: %s", video.Title)

	// Get formats
	// WithAudioChannels() only returns "muxed" formats (limited resolution, e.g. 720p)
	// For 1080p+, we need to download video and audio separately and merge.

	var bestVideo *youtube.Format
	var bestAudio *youtube.Format
	var bestMuxed *youtube.Format

	// Find best muxed (fallback)
	muxedFormats := video.Formats.WithAudioChannels()
	for i := range muxedFormats {
		f := &muxedFormats[i]
		if bestMuxed == nil || (f.Width*f.Height > bestMuxed.Width*bestMuxed.Height) {
			bestMuxed = f
		}
	}

	// Find best video-only
	videoFormats := video.Formats.Type("video/mp4")
	for i := range videoFormats {
		f := &videoFormats[i]
		// Skip if it has audio (those are muxed and handled above)
		if f.AudioChannels > 0 {
			continue
		}
		if bestVideo == nil || (f.Width*f.Height > bestVideo.Width*bestVideo.Height) {
			bestVideo = f
		}
	}

	// Find best audio-only
	audioFormats := video.Formats.Type("audio")
	for i := range audioFormats {
		f := &audioFormats[i]
		if bestAudio == nil || f.Bitrate > bestAudio.Bitrate {
			bestAudio = f
		}
	}

	// Determine strategy: If we have a video-only format that's better than muxed, use it and merge.
	useMerge := false
	if bestVideo != nil && bestMuxed != nil && (bestVideo.Width*bestVideo.Height > bestMuxed.Width*bestMuxed.Height) {
		useMerge = true
	} else if bestVideo != nil && bestMuxed == nil {
		useMerge = true
	}

	var finalPath string
	if useMerge && bestAudio != nil {
		logger.Infof("Selected separate streams for 1080p+: Video (%dx%d), Audio (%d bps)",
			bestVideo.Width, bestVideo.Height, bestAudio.Bitrate)

		// Download video stream
		vStream, _, err := client.GetStream(video, bestVideo)
		if err != nil {
			return "", "", fmt.Errorf("failed to get video stream: %w", err)
		}
		defer vStream.Close()
		vFile, _ := os.CreateTemp("", "yt_video_*.mp4")
		io.Copy(vFile, vStream)
		vFile.Close()
		defer os.Remove(vFile.Name())

		// Download audio stream
		aStream, _, err := client.GetStream(video, bestAudio)
		if err != nil {
			return "", "", fmt.Errorf("failed to get audio stream: %w", err)
		}
		defer aStream.Close()
		aFile, _ := os.CreateTemp("", "yt_audio_*.m4a")
		io.Copy(aFile, aStream)
		aFile.Close()
		defer os.Remove(aFile.Name())

		// Final output file
		outFile, err := os.CreateTemp("", "youtube_*.mp4")
		if err != nil {
			return "", "", fmt.Errorf("failed to create final temp file: %w", err)
		}
		finalPath = outFile.Name()
		outFile.Close()

		// Merge with ffmpeg
		logger.Infof("Merging streams with ffmpeg...")
		cmd := exec.Command("ffmpeg", "-y", "-i", vFile.Name(), "-i", aFile.Name(), "-c", "copy", finalPath)
		if err := cmd.Run(); err != nil {
			logger.Errorf("ffmpeg merge failed: %v. Falling back to muxed stream.", err)
			useMerge = false
			os.Remove(finalPath)
		} else {
			return finalPath, video.Title, nil
		}
	}

	// Fallback to muxed stream
	if bestMuxed == nil {
		return "", "", fmt.Errorf("no suitable formats found")
	}

	logger.Infof("Selected muxed format: %s (Quality: %dx%d, Bitrate: %d)",
		bestMuxed.MimeType, bestMuxed.Width, bestMuxed.Height, bestMuxed.Bitrate)

	// Get stream
	stream, _, err := client.GetStream(video, bestMuxed)
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
	defer tempFile.Close()

	// Download to temp file
	logger.Infof("Downloading YouTube video to temp file...")
	if _, err = io.Copy(tempFile, stream); err != nil {
		os.Remove(tempPath)
		return "", "", fmt.Errorf("failed to download video: %w", err)
	}

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

// LoadCookies parses a Netscape/curl format cookies file into a CookieJar
func LoadCookies(jar *cookiejar.Jar, filepath string) error {
	file, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) < 7 {
			continue
		}

		domain := parts[0]
		// path := parts[2]
		rawSecure := parts[3]
		expiresStr := parts[4]
		name := parts[5]
		value := parts[6]

		expires, _ := strconv.ParseInt(expiresStr, 10, 64)
		secure := strings.ToUpper(rawSecure) == "TRUE"

		// Create the cookie
		cookie := &http.Cookie{
			Name:    name,
			Value:   value,
			Domain:  domain,
			Path:    "/",
			Expires: time.Unix(expires, 0),
			Secure:  secure,
		}

		// Set the cookie for both http and https for the domain
		u, _ := url.Parse("https://" + strings.TrimPrefix(domain, "."))
		jar.SetCookies(u, []*http.Cookie{cookie})
		u2, _ := url.Parse("http://" + strings.TrimPrefix(domain, "."))
		jar.SetCookies(u2, []*http.Cookie{cookie})
	}

	return scanner.Err()
}
