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

	// Determine if cookies are available
	cookieFile := "youtube_cookies.txt"
	hasCookies := false
	if _, err := os.Stat(cookieFile); err == nil {
		hasCookies = true
	} else {
		absPath, _ := filepath.Abs(cookieFile)
		logger.Warnf("YouTube cookies file not found at %s. Age-restricted videos may fail.", absPath)
	}

	// Internal helper to create a client with specific settings
	createClient := func(userAgent string) *youtube.Client {
		c := &youtube.Client{}
		httpClient := &http.Client{}

		if hasCookies {
			jar, err := cookiejar.New(nil)
			if err != nil {
				logger.Warnf("Failed to create cookie jar: %v", err)
			} else if err := LoadCookies(jar, cookieFile); err == nil {
				httpClient.Jar = jar
				logger.Infof("Loaded YouTube cookies from %s", cookieFile)

				// Verify cookies for youtube.com
				u, _ := url.Parse("https://www.youtube.com")
				loaded := jar.Cookies(u)
				logger.Infof("Jar has %d cookies active for %s", len(loaded), u.Host)
			} else {
				logger.Warnf("Failed to load YouTube cookies from %s: %v", cookieFile, err)
			}
		}

		httpClient.Transport = &userAgentRoundTripper{
			RoundTripper: http.DefaultTransport,
			UserAgent:    userAgent,
		}
		c.HTTPClient = httpClient
		return c
	}

	// Try fetching video info with a standard client first
	client := createClient("")
	video, err := client.GetVideo(pageURL)
	if err != nil {
		logger.Warnf("Standard YouTube fetch failed: %v.", err)

		// If it's an age restriction or embedding error, try with a TV client User-Agent
		if strings.Contains(err.Error(), "age restriction") || strings.Contains(err.Error(), "embedding") {
			logger.Infof("Age restriction or embedding issue detected. Retrying with TV Client parameters...")
			tvClient := createClient("Mozilla/5.0 (SMART-TV; Linux; Tizen 2.4.0) AppleWebKit/538.1 (KHTML, like Gecko) SamsungBrowser/1.1 tv")
			video, err = tvClient.GetVideo(pageURL)
			if err != nil {
				logger.Errorf("TV Client retry also failed: %v", err)
				return "", "", fmt.Errorf("failed to get YouTube video info after retry: %w", err)
			}
			logger.Infof("TV Client retry successful.")
			client = tvClient // Use the successful client for subsequent operations
		} else {
			return "", "", fmt.Errorf("failed to get YouTube video info: %w", err)
		}
	}

	logger.Infof("YouTube video: %s", video.Title)

	// Quality Selection: Search FOR ALL formats (including DASH separate streams)
	// Resolution is usually better in separate video/audio streams (DASH)

	var bestVideo *youtube.Format
	var bestAudio *youtube.Format
	var bestMuxed *youtube.Format

	for i := range video.Formats {
		f := &video.Formats[i]

		// Case 1: Muxed (Video + Audio)
		if f.AudioChannels > 0 && f.Width > 0 {
			if bestMuxed == nil || (f.Width*f.Height > bestMuxed.Width*bestMuxed.Height) {
				bestMuxed = f
			}
		}

		// Case 2: Video Only
		if f.AudioChannels == 0 && f.Width > 0 {
			if bestVideo == nil || (f.Width*f.Height > bestVideo.Width*bestVideo.Height) {
				bestVideo = f
			}
		}

		// Case 3: Audio Only
		if f.AudioChannels > 0 && f.Width == 0 {
			if bestAudio == nil || f.Bitrate > bestAudio.Bitrate {
				bestAudio = f
			}
		}
	}

	// Determine strategy
	useMerge := false
	if bestVideo != nil && bestMuxed != nil && (bestVideo.Width*bestVideo.Height > bestMuxed.Width*bestMuxed.Height) {
		useMerge = true
	} else if bestVideo != nil && bestMuxed == nil {
		useMerge = true
	}

	if useMerge && bestAudio != nil {
		logger.Infof("Selected DASH streams: Video %dx%d (%s), Audio %d bps (%s)",
			bestVideo.Width, bestVideo.Height, bestVideo.MimeType, bestAudio.Bitrate, bestAudio.MimeType)

		// Helper to download a format to a temp file
		downloadToTemp := func(f *youtube.Format, suffix string) (string, error) {
			s, _, err := client.GetStream(video, f)
			if err != nil {
				return "", err
			}
			defer s.Close()

			tmp, err := os.CreateTemp("", "yt_part_*"+suffix)
			if err != nil {
				return "", err
			}
			// Close the file handle immediately so io.Copy can write to it and ffmpeg can access it later on Windows
			defer tmp.Close()

			if _, err := io.Copy(tmp, s); err != nil {
				os.Remove(tmp.Name())
				return "", err
			}
			return tmp.Name(), nil
		}

		vPath, err := downloadToTemp(bestVideo, ".mp4")
		if err != nil {
			return "", "", fmt.Errorf("failed to download video stream: %w", err)
		}
		defer os.Remove(vPath)

		aPath, err := downloadToTemp(bestAudio, ".m4a")
		if err != nil {
			return "", "", fmt.Errorf("failed to download audio stream: %w", err)
		}
		defer os.Remove(aPath)

		// Final output file
		finalTmp, err := os.CreateTemp("", "youtube_*.mp4")
		if err != nil {
			return "", "", fmt.Errorf("failed to create final temp file: %w", err)
		}
		finalPath := finalTmp.Name()
		finalTmp.Close() // Close immediately so ffmpeg can write to it

		logger.Infof("Merging DASH streams with ffmpeg...")
		cmd := exec.Command("ffmpeg", "-y", "-i", vPath, "-i", aPath, "-c", "copy", finalPath)
		if err := cmd.Run(); err != nil {
			logger.Errorf("ffmpeg merge failed: %v. Falling back to muxed stream.", err)
			os.Remove(finalPath)
			// Fall through to muxed fallback
		} else {
			return finalPath, video.Title, nil
		}
	}

	// Fallback to Muxed
	if bestMuxed == nil {
		return "", "", fmt.Errorf("no suitable YouTube formats found")
	}

	logger.Infof("Selected muxed format: %dx%d (%s)", bestMuxed.Width, bestMuxed.Height, bestMuxed.MimeType)

	stream, _, err := client.GetStream(video, bestMuxed)
	if err != nil {
		return "", "", fmt.Errorf("failed to get muxed stream: %w", err)
	}
	defer stream.Close()

	tempFile, err := os.CreateTemp("", "youtube_*.mp4")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()

	if _, err = io.Copy(tempFile, stream); err != nil {
		tempFile.Close() // Ensure file is closed on error
		os.Remove(tempPath)
		return "", "", fmt.Errorf("failed to download video: %w", err)
	}
	tempFile.Close() // Close before returning so caller can move it on Windows

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
