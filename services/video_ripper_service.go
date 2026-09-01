package services

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gallery_api/logger"
	"hash"
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
		"--socket-timeout", "30",
		"-o", outputPathTemplate,
	}

	cookieFile := "youtube_cookies.txt"
	if _, err := os.Stat(cookieFile); err == nil {
		args = append(args, "--cookies", cookieFile)
		logger.Infof("Using cookies from %s", cookieFile)
	}

	// First, let's get the title and the expected filename
	metadataArgs := append([]string{"--get-title", "--get-filename", "-o", outputPathTemplate}, args[3:]...) // skip -o and socket-timeout, replace -o for filename check
	metadataArgs = append(metadataArgs, pageURL)

	ctxMeta, cancelMeta := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelMeta()
	cmdMetadata := exec.CommandContext(ctxMeta, "yt-dlp", metadataArgs...)
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
	ctxDownload, cancelDownload := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancelDownload()
	cmdDownload := exec.CommandContext(ctxDownload, "yt-dlp", downloadArgs...)

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

const videoChunkSize = 8 * 1024 * 1024 // 8MB per chunk

// probeRangeSupport sends a HEAD request to check whether the server supports
// HTTP Range requests and returns the total file size.
func probeRangeSupport(ctx context.Context, client *http.Client, videoURL, pageURL string) (int64, bool) {
	req, err := http.NewRequestWithContext(ctx, "HEAD", videoURL, nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("Referer", pageURL)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101 Firefox/91.0")

	resp, err := client.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) // drain body so the connection can be reused

	if resp.StatusCode != 200 {
		return 0, false
	}

	acceptRanges := resp.Header.Get("Accept-Ranges")
	contentLength := resp.ContentLength

	return contentLength, acceptRanges == "bytes" && contentLength > 0
}

// downloadChunked downloads a file in sequential 8MB chunks using HTTP Range
// requests. Each chunk is written directly to the file and hashed
// incrementally, keeping memory usage constant regardless of total file size.
func downloadChunked(ctx context.Context, client *http.Client, videoURL, pageURL string, dst *os.File, hasher hash.Hash, totalSize int64) error {
	var offset int64
	buf := make([]byte, 32*1024) // 32KB read buffer, reused across all chunks

	for offset < totalSize {
		end := offset + videoChunkSize - 1
		if end >= totalSize {
			end = totalSize - 1
		}

		if err := downloadChunkWithRetry(ctx, client, videoURL, pageURL, dst, hasher, offset, end, buf); err != nil {
			return fmt.Errorf("chunk %d-%d failed: %w", offset, end, err)
		}

		offset = end + 1

		// Log progress periodically (every ~50MB)
		if offset%(50*1024*1024) < videoChunkSize {
			pct := float64(offset) / float64(totalSize) * 100
			logger.Infof("Download progress: %.1f%% (%s / %s)", pct, formatBytes(offset), formatBytes(totalSize))
		}
	}

	return nil
}

// downloadChunkWithRetry downloads a single byte range with up to 3 retries
// and exponential backoff. Requires a 206 Partial Content response and
// validates the Content-Range header to detect servers that ignore the Range
// header and send the full body.
func downloadChunkWithRetry(ctx context.Context, client *http.Client, videoURL, pageURL string, dst *os.File, hasher hash.Hash, start, end int64, buf []byte) error {
	const maxRetries = 3
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			logger.Debugf("Retrying chunk %d-%d (attempt %d) after %v", start, end, attempt+1, backoff)
			time.Sleep(backoff)
		}

		req, err := http.NewRequestWithContext(ctx, "GET", videoURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
		req.Header.Set("Referer", pageURL)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101 Firefox/91.0")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		// Require 206 Partial Content. If the server returns 200 OK it means
		// the Range header was ignored and the full body is being sent, which
		// would blow up disk usage and waste bandwidth.
		if resp.StatusCode != 206 {
			resp.Body.Close()
			lastErr = fmt.Errorf("expected 206 got %d for range %d-%d (server ignoring Range header)", resp.StatusCode, start, end)
			continue
		}

		// Validate Content-Range header to confirm the server sent the
		// exact byte range we requested.
		cr := resp.Header.Get("Content-Range")
		if cr == "" {
			resp.Body.Close()
			lastErr = fmt.Errorf("missing Content-Range header in 206 response for %d-%d", start, end)
			continue
		}
		expectedRange := fmt.Sprintf("bytes %d-%d", start, end)
		if !strings.HasPrefix(cr, expectedRange) {
			resp.Body.Close()
			lastErr = fmt.Errorf("Content-Range mismatch: expected prefix %q, got %q", expectedRange, cr)
			continue
		}

		// Write chunk data directly to file and hash simultaneously.
		// The read is bounded by the chunk size; io.CopyBuffer returns
		// after resp.Body.Read returns EOF, which the server signals
		// after sending exactly the requested byte range.
		n, err := io.CopyBuffer(io.MultiWriter(dst, hasher), resp.Body, buf)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		expectedBytes := end - start + 1
		if n != expectedBytes {
			// Truncate the file back to where the chunk started so
			// we don't leave partial garbage on failure.
			dst.Truncate(start)
			dst.Seek(start, io.SeekStart)
			lastErr = fmt.Errorf("chunk %d-%d: wrote %d bytes, expected %d", start, end, n, expectedBytes)
			continue
		}

		return nil
	}

	return fmt.Errorf("chunk download failed after %d retries: %w", maxRetries, lastErr)
}

// downloadStreaming performs a standard streaming download as a fallback
// when the server does not support HTTP Range requests. Uses a buffered
// writer to reduce syscall overhead.
func downloadStreaming(ctx context.Context, client *http.Client, videoURL, pageURL string, dst *os.File, hasher hash.Hash) error {
	req, err := http.NewRequestWithContext(ctx, "GET", videoURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Referer", pageURL)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101 Firefox/91.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading video: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("video download returned status %d", resp.StatusCode)
	}

	// Use a buffered writer (256KB) to reduce syscall overhead for large files
	bufWriter := bufio.NewWriterSize(dst, 256*1024)
	multiWriter := io.MultiWriter(bufWriter, hasher)
	if _, err := io.Copy(multiWriter, resp.Body); err != nil {
		return fmt.Errorf("writing video data: %w", err)
	}
	if err := bufWriter.Flush(); err != nil {
		return fmt.Errorf("flushing video data: %w", err)
	}

	return nil
}

// formatBytes formats a byte count as a human-readable string.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// DownloadVideo downloads a video from a direct URL and saves it with hash-based naming.
// It uses chunked HTTP Range requests (8MB chunks) when the server supports them,
// falling back to buffered streaming otherwise. This keeps memory usage constant
// regardless of video file size.
func DownloadVideo(videoURL string, sourceName string, pageURL string, title string) (*DownloadImageResult, error) {
	logger.Infof("Downloading video from %s", videoURL)

	// YouTube pages / short links can't be streamed directly — rip through yt-dlp
	if isYouTubeURL(videoURL) {
		logger.Infof("Detected YouTube URL, invoking RipYouTube...")
		localPath, ytTitle, err := RipYouTube(videoURL)
		if err != nil {
			return nil, fmt.Errorf("failed to download YouTube video: %w", err)
		}
		if title == "" && ytTitle != "" {
			title = ytTitle
		}
		result, err := ImportLocalVideo(localPath, sourceName)
		if err != nil {
			return nil, fmt.Errorf("failed to import YouTube video: %w", err)
		}
		result.Title = title
		logger.Infof("Saved YouTube video to: %s", result.Path)
		return result, nil
	}

	// Use the appropriate HTTP client (WireGuard for blocked domains, default otherwise)
	client := GetHTTPClient(videoURL)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Determine extension
	ext := ".mp4"
	if strings.HasSuffix(strings.ToLower(videoURL), ".webm") {
		ext = ".webm"
	} else if strings.HasSuffix(strings.ToLower(videoURL), ".mkv") {
		ext = ".mkv"
	}

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

	// Create temp file for download
	tmpFile, err := os.CreateTemp(fullDir, "video-*"+ext)
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	hasher := sha256.New()

	// Probe the server for Range request support and total file size
	totalSize, supportsRange := probeRangeSupport(ctx, client, videoURL, pageURL)

	if supportsRange && totalSize > 0 {
		// Chunked download using HTTP Range requests
		logger.Infof("Server supports range requests, downloading %s in 8MB chunks", formatBytes(totalSize))
		if err := downloadChunked(ctx, client, videoURL, pageURL, tmpFile, hasher, totalSize); err != nil {
			logger.Warnf("Chunked download failed (%v), falling back to streaming", err)
			// Reset temp file and hasher for streaming fallback
			tmpFile.Truncate(0)
			tmpFile.Seek(0, io.SeekStart)
			hasher = sha256.New()
			if err := downloadStreaming(ctx, client, videoURL, pageURL, tmpFile, hasher); err != nil {
				tmpFile.Close()
				return nil, fmt.Errorf("streaming download failed after chunked fallback: %w", err)
			}
		}
	} else {
		// Fallback: standard streaming download with buffered I/O
		logger.Info("Server does not support range requests, using buffered streaming download")
		if err := downloadStreaming(ctx, client, videoURL, pageURL, tmpFile, hasher); err != nil {
			tmpFile.Close()
			return nil, fmt.Errorf("streaming download failed: %w", err)
		}
	}

	tmpFile.Close()

	// Verify we got data
	info, err := os.Stat(tmpPath)
	if err != nil || info.Size() == 0 {
		return nil, fmt.Errorf("downloaded video is empty")
	}

	hashStr := hex.EncodeToString(hasher.Sum(nil))
	filename := hashStr + ext
	destPath := filepath.Join(fullDir, filename)

	// If final file already exists, remove temp and return
	if _, err := os.Stat(destPath); err == nil {
		logger.Debugf("Video file already exists: %s", destPath)
		return buildVideoResult(destPath, title)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return nil, fmt.Errorf("renaming video file: %w", err)
	}
	logger.Infof("Saved video to: %s (%s)", destPath, formatBytes(info.Size()))

	return buildVideoResult(destPath, title)
}

// buildVideoResult generates thumbnail, trickplay, metadata for a video and returns the result
func buildVideoResult(destPath string, title string) (*DownloadImageResult, error) {
	if _, err := GenerateVideoThumbnail(destPath); err != nil {
		logger.Warnf("Failed to generate video thumbnail: %v", err)
	}

	if err := GenerateTrickplayData(destPath); err != nil {
		logger.Warnf("Failed to generate trickplay data: %v", err)
	}

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
		DominantColors: "[]",
	}, nil
}

// extractVideoIDFromPMVHavenURL extracts the 24-character hex video ID from a PMVHaven URL.
// URL format: https://pmvhaven.com/video/Slug_Name_66195f01d0f2168854325fd0
// URLs from search results commonly carry query strings (e.g. ?from=search&q=...), so
// the query string is stripped before matching.
func extractVideoIDFromPMVHavenURL(pageURL string) (string, error) {
	if u, err := url.Parse(pageURL); err == nil {
		u.RawQuery = ""
		u.Fragment = ""
		pageURL = u.String()
	}
	re := regexp.MustCompile(`_([a-f0-9]{24})/?$`)
	matches := re.FindStringSubmatch(pageURL)
	if len(matches) < 2 {
		return "", fmt.Errorf("could not extract video ID from URL: %s", pageURL)
	}
	return matches[1], nil
}

// nuxtDataLookup resolves a "pointer" value in the Nuxt data array.
// In Nuxt's payload format, some values are stored as integers that index into
// the top-level array. If val is an int, we look up data[val]; otherwise we
// return val unchanged.
func nuxtDataLookup(data []interface{}, val interface{}) interface{} {
	if idx, ok := val.(float64); ok {
		i := int(idx)
		if i >= 0 && i < len(data) {
			return data[i]
		}
	}
	return val
}

// nuxtDataResolveString resolves a string pointer from the Nuxt data array.
func nuxtDataResolveString(data []interface{}, val interface{}) string {
	resolved := nuxtDataLookup(data, val)
	if s, ok := resolved.(string); ok {
		return s
	}
	return ""
}

// nuxtDataResolveMap resolves a map pointer from the Nuxt data array.
func nuxtDataResolveMap(data []interface{}, val interface{}) map[string]interface{} {
	resolved := nuxtDataLookup(data, val)
	if m, ok := resolved.(map[string]interface{}); ok {
		return m
	}
	return nil
}

// findStringInNuxtData walks the Nuxt data array looking for a string value
// that matches a predicate. This is used to find video URLs buried in the
// pointer-based structure.
func isVideoURL(s string) bool {
	return strings.HasPrefix(s, "http") &&
		(strings.HasSuffix(s, ".mp4") || strings.HasSuffix(s, ".m3u8")) &&
		!strings.Contains(s, "/videoPreview/") &&
		!strings.Contains(s, "/thumbnail/") &&
		!strings.Contains(s, "/previews/")
}

func findURLInNuxtData(data []interface{}) string {
	// Walk all entries looking for URLs ending in .mp4 or .m3u8 that look like
	// actual download/stream URLs (not preview thumbnails).
	for _, item := range data {
		switch v := item.(type) {
		case string:
			if isVideoURL(v) {
				return v
			}
		case map[string]interface{}:
			for _, field := range v {
				if s, ok := field.(string); ok {
					if isVideoURL(s) {
						return s
					}
				}
			}
		}
	}
	return ""
}

// pmvhavenAPIVideoInput calls the PMVHaven v2 API to get video details including
// the download URL. Requires authentication cookies.
func pmvhavenAPIVideoInput(videoID string, client *http.Client) (string, string, error) {
	apiURL := "https://pmvhaven.com/api/v2/videoInput"
	bodyStr := fmt.Sprintf(`{"mode":"getVideo","id":"%s"}`, videoID)

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(bodyStr))
	if err != nil {
		return "", "", fmt.Errorf("creating API request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://pmvhaven.com/")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return "", "", fmt.Errorf("API returned 401 (unauthorized) — session cookies required for PMVHaven downloads; provide a cookies.txt file")
	}
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result struct {
		Video struct {
			URL   string `json:"url"`
			Title string `json:"title"`
		} `json:"video"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("decoding API response: %w", err)
	}

	if result.Video.URL == "" {
		return "", "", fmt.Errorf("API response missing video URL")
	}

	return result.Video.URL, result.Video.Title, nil
}

// pmvhavenWatchPageVideo calls the PMVHaven watch-page API to get video details
// including the direct video URL. This is the same endpoint the site's own
// Nuxt client uses, and it returns public video URLs without authentication.
func pmvhavenWatchPageVideo(videoID string, client *http.Client) (string, string, error) {
	apiURL := fmt.Sprintf("https://pmvhaven.com/api/videos/%s/watch-page", videoID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("creating API request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://pmvhaven.com/")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", "", fmt.Errorf("Video not found")
	}
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Video struct {
				Title                string `json:"title"`
				URL                  string `json:"videoUrl"`
				HLSMasterPlaylistURL string `json:"hlsMasterPlaylistUrl"`
				HLSEnabled           bool   `json:"hlsEnabled"`
			} `json:"video"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("decoding API response: %w", err)
	}

	if !result.Success || result.Data.Video.URL == "" {
		return "", "", fmt.Errorf("API response missing video URL")
	}

	return result.Data.Video.URL, result.Data.Video.Title, nil
}

// RipPMVHaven extracts the video URL from a PMVHaven page.
//
// PMVHaven was rebuilt as a Nuxt.js SPA (Nov 2025). Video download URLs are no
// longer embedded in the HTML — they require an authenticated API call or a
// click-to-reveal interaction. This function uses multiple strategies:
//
//  1. Query the PMVHaven watch-page API (/api/videos/{id}/watch-page). This is
//     the endpoint the site's own client uses and it returns the direct video
//     URL and title for public videos without any authentication.
//  2. Parse __NUXT_DATA__ from the page to extract video metadata and try to
//     find any embedded download or HLS stream URL (.mp4 / .m3u8)
//  3. If a pmvhaven_cookies.txt file exists, use it to authenticate with the
//     PMVHaven v2 API and retrieve the download URL
//  4. Fall back to og:video / og:video:url meta tags
func RipPMVHaven(pageURL string) (string, string, error) {
	logger.Infof("Starting RipPMVHaven for %s", pageURL)

	videoID, err := extractVideoIDFromPMVHavenURL(pageURL)
	if err != nil {
		logger.Warnf("Could not extract video ID from URL, proceeding with fallback methods: %v", err)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Strategy 1: watch-page API (no auth required for public videos)
	if videoID != "" {
		apiURL, apiTitle, apiErr := pmvhavenWatchPageVideo(videoID, client)
		if apiErr == nil {
			logger.Infof("Successfully retrieved video URL from watch-page API")
			return apiURL, apiTitle, nil
		}
		logger.Warnf("watch-page API call failed: %v", apiErr)
		if strings.Contains(apiErr.Error(), "Video not found") {
			return "", "", fmt.Errorf("PMVHaven video %s does not appear to exist anymore (it may have been removed): %v", videoID, apiErr)
		}
	}

	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("fetching page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("page returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("reading page body: %w", err)
	}
	bodyStr := string(bodyBytes)

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(bodyStr))
	if err != nil {
		return "", "", fmt.Errorf("parsing HTML: %w", err)
	}

	// Extract title from <title> tag (h1 on the SPA shell stays "Dashboard")
	title := strings.TrimSpace(doc.Find("title").First().Text())
	title = strings.TrimPrefix(title, "Loading...")
	title = strings.TrimSuffix(title, " - PMVHaven")
	if title == "" {
		title, _ = doc.Find("meta[property='og:title']").Attr("content")
		title = strings.TrimSuffix(strings.TrimSpace(title), " - PMVHaven")
	}
	if title == "" {
		title = "Unknown Video"
	}

	// Try to extract the __NUXT_DATA__ payload from a <script> tag with id="__NUXT_DATA__"
	var nuxtData []interface{}
	var videoURL string

	doc.Find("script#__NUXT_DATA__").Each(func(i int, s *goquery.Selection) {
		if videoURL != "" {
			return
		}
		raw := s.Text()
		if raw == "" {
			return
		}
		// Unescape JS-escaped Unicode sequences
		raw = strings.NewReplacer(
			`\/`, "/",
			`\u002F`, "/",
			`\u0026`, "&",
			`\u003C`, "<",
			`\u003E`, ">",
			`\u0022`, `"`,
		).Replace(raw)

		if err := json.Unmarshal([]byte(raw), &nuxtData); err != nil {
			logger.Warnf("Failed to parse __NUXT_DATA__ JSON: %v", err)
			return
		}
		logger.Infof("Parsed __NUXT_DATA__ array with %d elements", len(nuxtData))

		// Try to find any download URLs in the raw text first
		reURL := regexp.MustCompile(`https?://[^"'\s\\]+\.(mp4|m3u8)`)
		for _, match := range reURL.FindAllString(raw, -1) {
			if !strings.Contains(match, "/videoPreview/") && !strings.Contains(match, "/thumbnail/") && !strings.Contains(match, "/previews/") {
				videoURL = match
				logger.Infof("Found video URL in Nuxt data: %s", videoURL)
				return
			}
		}

		// Walk the Nuxt data array for eligible URLs
		if found := findURLInNuxtData(nuxtData); found != "" {
			videoURL = found
			logger.Infof("Found download URL by walking Nuxt data: %s", videoURL)
			return
		}

		// Try to resolve the video metadata through Nuxt pointers
		// Nuxt data[1] typically contains the page store data
		if len(nuxtData) > 1 {
			pageData := nuxtDataResolveMap(nuxtData, nuxtData[1])
			if pageData != nil {
				if dataRef, ok := pageData["data"]; ok {
					dataMap := nuxtDataResolveMap(nuxtData, dataRef)
					if dataMap != nil {
						// Look for a key containing "video"
						for k, v := range dataMap {
							if strings.Contains(k, "video") || strings.Contains(k, "Video") {
								videoMap := nuxtDataResolveMap(nuxtData, v)
								if videoMap == nil {
									continue
								}
								// Try to get URL from the video object
								for _, field := range []string{"url", "sourceUrl", "downloadUrl", "file", "src"} {
									if u, ok := videoMap[field]; ok {
										if urlStr := nuxtDataResolveString(nuxtData, u); urlStr != "" && strings.HasPrefix(urlStr, "http") {
											videoURL = urlStr
											logger.Infof("Found video URL via Nuxt pointer resolution: %s", videoURL)
											return
										}
									}
								}
							}
						}
					}
				}
			}
		}
	})

	// Try authenticated API if available (cookies file provides session auth)
	if videoURL == "" {
		cookieFile := "pmvhaven_cookies.txt"
		if vidID := videoID; vidID != "" {
			if _, err := os.Stat(cookieFile); err == nil {
				logger.Infof("Found %s, attempting authenticated API call", cookieFile)
				jar, _ := cookiejar.New(nil)
				if loadErr := LoadCookies(jar, cookieFile); loadErr == nil {
					apiClient := &http.Client{
						Jar:     jar,
						Timeout: 30 * time.Second,
					}
					// Apply the cookie jar to the pmvhaven domain
					u, _ := url.Parse("https://pmvhaven.com")
					jar.SetCookies(u, jar.Cookies(u))

					apiURL, apiTitle, apiErr := pmvhavenAPIVideoInput(vidID, apiClient)
					if apiErr == nil {
						logger.Infof("Successfully retrieved video URL from API")
						if title == "Unknown Video" && apiTitle != "" {
							title = apiTitle
						}
						return apiURL, title, nil
					}
					logger.Warnf("API call failed: %v", apiErr)
				} else {
					logger.Warnf("Failed to load cookie file %s: %v", cookieFile, loadErr)
				}
			} else {
				logger.Debugf("No %s found (video ID: %s), API auth not available", cookieFile, vidID)
			}
		}
	}

	// Fallback: check standard meta tags
	if videoURL == "" {
		videoURL, _ = doc.Find("meta[property='og:video']").Attr("content")
		if videoURL == "" {
			videoURL, _ = doc.Find("meta[property='og:video:url']").Attr("content")
		}
		if videoURL != "" && !strings.HasSuffix(videoURL, ".mp4") && !strings.HasSuffix(videoURL, ".m3u8") {
			logger.Debugf("Ignoring non-video og:video: %s", videoURL)
			videoURL = ""
		}
		if videoURL != "" {
			logger.Infof("Found video URL via og:video meta: %s", videoURL)
		}
	}

	// Fallback: check for video source tags
	if videoURL == "" {
		doc.Find("video source").Each(func(i int, s *goquery.Selection) {
			if src, exists := s.Attr("src"); exists && (strings.Contains(src, ".mp4") || strings.Contains(src, ".m3u8")) {
				videoURL = src
				logger.Debugf("Found video source in HTML5 tag: %s", videoURL)
			}
		})
	}

	if videoURL == "" {
		return "", "", fmt.Errorf("could not find video URL on %s — PMVHaven may have removed the video or changed their layout", pageURL)
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
