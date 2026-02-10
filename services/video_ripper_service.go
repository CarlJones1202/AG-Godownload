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
)

// RipYouTube downloads a YouTube video using yt-dlp and returns the file path and title
func RipYouTube(pageURL string) (string, string, error) {
	logger.Infof("Starting RipYouTube with yt-dlp for %s", pageURL)

	// Final output file - we'll let yt-dlp write directly to a temp file
	tempDir := os.TempDir()
	outputPathTemplate := filepath.Join(tempDir, "yt_dlp_%(id)s.%(ext)s")

	// Prepare yt-dlp command
	// -f "bestvideo+bestaudio/best" : Select best quality and merge
	// --get-title : We'll need another call or use --print to get metadata
	// --cookies : Use existing cookies if available
	// -o : Output template
	// --no-playlist : Just the video

	args := []string{
		"-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best",
		"--no-playlist",
		"--merge-output-format", "mp4",
		"-o", outputPathTemplate,
	}

	cookieFile := "youtube_cookies.txt"
	if _, err := os.Stat(cookieFile); err == nil {
		args = append(args, "--cookies", cookieFile)
		logger.Infof("Using cookies from %s", cookieFile)
	}

	// First, let's get the title and the expected filename
	metadataArgs := append([]string{"--get-title", "--get-filename", "-o", outputPathTemplate}, args[2:]...) // replace -o for filename check
	metadataArgs = append(metadataArgs, pageURL)

	cmdMetadata := exec.Command("yt-dlp", metadataArgs...)
	output, err := cmdMetadata.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			logger.Errorf("yt-dlp metadata error: %s", string(exitErr.Stderr))
		}
		return "", "", fmt.Errorf("failed to get metadata from yt-dlp: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return "", "", fmt.Errorf("unexpected output from yt-dlp metadata: %s", string(output))
	}
	title := lines[0]
	actualPath := lines[1]

	logger.Infof("YouTube video: %s (Expected Path: %s)", title, actualPath)

	// Now perform the actual download
	downloadArgs := append(args, pageURL)
	cmdDownload := exec.Command("yt-dlp", downloadArgs...)

	// Stream output to logs for visibility
	stderr, _ := cmdDownload.StderrPipe()
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			logger.Debugf("yt-dlp: %s", scanner.Text())
		}
	}()

	if err := cmdDownload.Run(); err != nil {
		return "", "", fmt.Errorf("yt-dlp download failed: %w", err)
	}

	// Verify the file exists
	if _, err := os.Stat(actualPath); err != nil {
		return "", "", fmt.Errorf("downloaded file not found at %s: %w", actualPath, err)
	}

	return actualPath, title, nil
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

type userAgentRoundTripper struct {
	http.RoundTripper
	UserAgent string
}

func (rt *userAgentRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.UserAgent != "" {
		req.Header.Set("User-Agent", rt.UserAgent)
	}

	// Debug: Log if cookies are being sent
	if cookies := req.Header.Get("Cookie"); cookies != "" {
		logger.Debugf("Sending cookies to %s: %s...", req.URL.Host, cookies[:strings.Index(cookies, "=")+5])
	} else {
		// If no Cookie header, check if Jar has them
		if rt.RoundTripper == nil {
			// fallback
		}
	}

	return http.DefaultTransport.RoundTrip(req)
}

// LoadCookies parses a Netscape/curl format cookies file into a CookieJar
func LoadCookies(jar *cookiejar.Jar, filepath string) error {
	file, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		// Use regex for more robust splitting (any whitespace/tab)
		re := regexp.MustCompile(`\s+`)
		parts := re.Split(line, -1)
		if len(parts) < 7 {
			continue
		}

		domain := parts[0]
		// rawDomainUsed := parts[1]
		// path := parts[2]
		rawSecure := parts[3]
		expiresStr := parts[4]
		name := parts[5]
		value := parts[6]

		expires, _ := strconv.ParseInt(expiresStr, 10, 64)
		secure := strings.ToUpper(rawSecure) == "TRUE"

		// Create the cookie
		cookie := &http.Cookie{
			Name:   name,
			Value:  value,
			Domain: domain,
			Path:   "/",
			Secure: secure,
		}

		// Handle session cookies (expiry 0 in Netscape format)
		if expires > 0 {
			cookie.Expires = time.Unix(expires, 0)
		}

		// Set the cookie for common YouTube domains to be safe
		domains := []string{
			strings.TrimPrefix(domain, "."),
			"youtube.com",
			"www.youtube.com",
			"m.youtube.com",
			"googlevideo.com",
		}

		for _, d := range domains {
			u, _ := url.Parse("https://" + d)
			jar.SetCookies(u, []*http.Cookie{cookie})
		}
		count++
	}
	logger.Infof("Successfully parsed %d cookies from file", count)

	return scanner.Err()
}
