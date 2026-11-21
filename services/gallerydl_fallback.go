package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"gallery_api/config"
	"gallery_api/logger"
	"io"
	urlpkg "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/disintegration/imaging"
)

// DownloadImageWithGalleryDL attempts to download the given URL using the
// gallery-dl external tool. It's intended as a fallback for hosts like imx.to
// where a normal HTTP client may return 503 or other bot-blocking behavior.
func DownloadImageWithGalleryDL(ctx context.Context, url string, sourceName string, timeout time.Duration) (*DownloadImageResult, error) {
	return downloadImageWithGalleryDL(ctx, url, sourceName, timeout, true)
}

// DownloadImageWithGalleryDLWithoutPostProcessing behaves like
// DownloadImageWithGalleryDL but skips dominant-color extraction and thumbnail
// generation so bulk recovery can defer that CPU-bound work to a
// post-processing pool. The returned DominantColors is empty; callers must
// compute it before persisting.
func DownloadImageWithGalleryDLWithoutPostProcessing(ctx context.Context, url string, sourceName string, timeout time.Duration) (*DownloadImageResult, error) {
	return downloadImageWithGalleryDL(ctx, url, sourceName, timeout, false)
}

func DownloadImageWithIMXFallback(ctx context.Context, originalURL, directURL, sourceName string) (*DownloadImageResult, error) {
	return downloadImageWithIMXFallback(ctx, originalURL, directURL, sourceName, true)
}

func DownloadImageWithIMXFallbackWithoutPostProcessing(ctx context.Context, originalURL, directURL, sourceName string) (*DownloadImageResult, error) {
	return downloadImageWithIMXFallback(ctx, originalURL, directURL, sourceName, false)
}

func downloadImageWithIMXFallback(ctx context.Context, originalURL, directURL, sourceName string, postProcess bool) (*DownloadImageResult, error) {
	pageURL := imxPageReferer(originalURL, directURL)
	resolvedURL := ""
	var resolveErr error
	if pageURL != "" {
		resolvedURL, resolveErr = RipImx(pageURL)
	}

	candidates := make([]string, 0, 2)
	if resolvedURL != "" {
		candidates = append(candidates, resolvedURL)
	}
	directPageURL := deriveIMXPageURL("", directURL)
	if directURL != "" && (resolvedURL == "" || directPageURL != pageURL) {
		candidates = append(candidates, directURL)
	}

	var lastErr error
	for _, candidate := range candidates {
		referer := pageURL
		if referer == "" {
			if u, err := urlpkg.Parse(candidate); err == nil {
				referer = u.Scheme + "://" + u.Host
			}
		}

		var result *DownloadImageResult
		if postProcess {
			result, lastErr = DownloadImage(candidate, sourceName, referer)
		} else {
			result, lastErr = DownloadImageWithoutColors(candidate, sourceName, referer)
		}
		if lastErr == nil {
			return result, nil
		}
		logger.Warnf("[imx] Direct fallback candidate failed for %s: %v", candidate, lastErr)
	}

	if len(candidates) == 0 && config.Global.GalleryDL.Enabled && pageURL != "" {
		result, err := downloadImageWithGalleryDL(ctx, pageURL, sourceName, time.Duration(config.Global.GalleryDL.TimeoutSec)*time.Second, postProcess)
		if err == nil {
			return result, nil
		}
		if resolveErr != nil {
			return nil, fmt.Errorf("could not resolve IMX image (%v); gallery-dl fallback failed: %w", resolveErr, err)
		}
		return nil, err
	}
	if lastErr != nil {
		return nil, fmt.Errorf("IMX image download failed: %w", lastErr)
	}
	if resolveErr != nil {
		return nil, fmt.Errorf("could not resolve IMX image: %w", resolveErr)
	}
	return nil, errors.New("no IMX page or direct image URL available")
}

var imxDirectURLPattern = regexp.MustCompile(`(?i)https?://(?:[^/]+\.)?imx\.to/(?:u/)?i/\d{4}/\d{2}/\d{2}/([^/.]+)\.(?:jpg|jpeg|png|gif|webp)`)
var imxPageURLPattern = regexp.MustCompile(`(?i)^/i/[^/]+/?$`)

func imxPageReferer(originalURL, directURL string) string {
	if parsed, err := urlpkg.Parse(originalURL); err == nil && isIMXHost(parsed.Hostname()) && imxPageURLPattern.MatchString(parsed.Path) {
		return originalURL
	}
	return deriveIMXPageURL(originalURL, directURL)
}

func imageDownloadReferer(provider, originalURL, directURL string) string {
	switch provider {
	case "imx":
		return imxPageReferer(originalURL, directURL)
	case "imagetwist":
		return originalURL
	default:
		return ""
	}
}

func deriveIMXPageURL(originalURL, directURL string) string {
	for _, candidate := range []string{originalURL, directURL} {
		if candidate == "" {
			continue
		}
		if matches := imxDirectURLPattern.FindStringSubmatch(candidate); len(matches) == 2 {
			return "https://imx.to/i/" + matches[1]
		}
	}
	return ""
}

func downloadImageWithGalleryDL(ctx context.Context, url string, sourceName string, timeout time.Duration, postProcess bool) (*DownloadImageResult, error) {
	if !config.Global.GalleryDL.Enabled {
		return nil, errors.New("gallery-dl fallback disabled in config")
	}

	bin := config.Global.GalleryDL.BinaryPath
	if bin == "" {
		bin = "gallery-dl"
	}

	command, argsPrefix, err := resolveGalleryDLCommand(bin)
	if err != nil {
		return nil, err
	}

	// Create temp dir for download
	tmpDir, err := os.MkdirTemp("", "gallerydl-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Build command. Use --no-part to avoid .part files and --range 1-1 to limit to the first item.
	// We run in the tempDir so files are created there.
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := append(append([]string{}, argsPrefix...), "--no-part", "--range", "1-1", url)
	cmd := exec.CommandContext(cmdCtx, command, args...)
	cmd.Dir = tmpDir

	out, err := cmd.CombinedOutput()
	if err != nil {
		// include a truncated output for debugging
		msg := string(out)
		if len(msg) > 4096 {
			msg = msg[:4096]
		}
		return nil, fmt.Errorf("gallery-dl failed: %v; output=%s", err, msg)
	}

	// Find candidate files in tmpDir (ignore JSON and info files)
	var bestPath string
	var bestSize int64
	entries, derr := os.ReadDir(tmpDir)
	if derr != nil {
		return nil, fmt.Errorf("failed to read gallery-dl temp dir: %w", derr)
	}

	for _, e := range entries {
		if e.IsDir() {
			// inspect nested directory
			subEntries, _ := os.ReadDir(filepath.Join(tmpDir, e.Name()))
			for _, se := range subEntries {
				if se.IsDir() {
					continue
				}
				name := se.Name()
				if strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".info") {
					continue
				}
				full := filepath.Join(tmpDir, e.Name(), name)
				fi, err := os.Stat(full)
				if err != nil {
					continue
				}
				if fi.Size() > bestSize {
					bestSize = fi.Size()
					bestPath = full
				}
			}
			continue
		}

		name := e.Name()
		if strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".info") {
			continue
		}
		full := filepath.Join(tmpDir, name)
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if fi.Size() > bestSize {
			bestSize = fi.Size()
			bestPath = full
		}
	}

	if bestPath == "" {
		return nil, fmt.Errorf("gallery-dl did not produce any downloadable files; raw output: %s", strings.TrimSpace(string(out)))
	}

	// Determine extension (prefer existing extension)
	ext := strings.ToLower(filepath.Ext(bestPath))
	if ext == "" {
		ext = ".jpg"
	}

	// Prepare destination
	sourceDir := SanitizeDirectoryName(sourceName)
	if sourceDir == "" {
		sourceDir = "unknown"
	}
	fullDir := filepath.Join(UploadsDir, sourceDir)
	if err := os.MkdirAll(fullDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create upload dir: %w", err)
	}

	// Stream-copy from gallery-dl output to destination while hashing
	destPath := filepath.Join(fullDir, "tmp-*"+ext)
	dst, err := os.CreateTemp(fullDir, "gdl-*"+ext)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := dst.Name()
	defer os.Remove(tmpPath)

	f, err := os.Open(bestPath)
	if err != nil {
		dst.Close()
		return nil, fmt.Errorf("failed to open downloaded file: %w", err)
	}
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(dst, hash), f); err != nil {
		dst.Close()
		return nil, fmt.Errorf("failed to copy downloaded file: %w", err)
	}
	dst.Close()

	hashStr := hex.EncodeToString(hash.Sum(nil))
	finalPath := filepath.Join(fullDir, hashStr+ext)

	// Reject placeholder or HTML responses before they replace a missing image.
	if isPlaceholder, reason, checkErr := checkHotlinkPlaceholder(tmpPath, "imx", bestSize, hashStr); checkErr == nil && isPlaceholder {
		_ = dst.Close()
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("gallery-dl downloaded hotlinking placeholder: %s", reason)
	} else if checkErr != nil {
		logger.Warnf("gallery-dl: failed to check placeholder status for %s: %v", tmpPath, checkErr)
	}

	// If final file exists already, remove temp and return
	if _, err := os.Stat(finalPath); err == nil {
		return &DownloadImageResult{
			Path: finalPath,
		}, nil
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		return nil, fmt.Errorf("failed to rename downloaded file: %w", err)
	}

	destPath = finalPath

	// Validate image
	if _, err := imaging.Open(destPath); err != nil {
		// remove invalid file
		_ = os.Remove(destPath)
		return nil, fmt.Errorf("gallery-dl downloaded invalid image: %w; rawOutput=%s", err, strings.TrimSpace(string(out)))
	}

	// Extract colors and generate thumbnail unless the caller deferred that
	// work to a post-processing pool (bulk recovery path).
	colors := ""
	if postProcess {
		var cerr error
		colors, cerr = ExtractDominantColors(destPath)
		if cerr != nil {
			colors = "[]"
		}
		if _, terr := GenerateThumbnail(destPath); terr != nil {
			logger.Warnf("gallery-dl: failed to generate thumbnail for %s: %v", destPath, terr)
		}
	}

	logger.Debugf("gallery-dl: downloaded %s -> %s (size=%d) in tempdir %s", url, destPath, bestSize, tmpDir)

	return &DownloadImageResult{
		Path:           destPath,
		DominantColors: colors,
	}, nil
}

func resolveGalleryDLCommand(configured string) (string, []string, error) {
	candidates := []string{configured}
	if configured == "" || configured == "gallery-dl" {
		candidates = append(candidates,
			"gallery-dl.exe",
			filepath.Join(os.Getenv("APPDATA"), "Python", "Python313", "Scripts", "gallery-dl.exe"),
		)
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved, nil, nil
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil, nil
		}
	}

	for _, python := range []string{
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Python", "Python313", "python.exe"),
		"py",
		"python",
	} {
		if python == "" {
			continue
		}
		resolved, err := exec.LookPath(python)
		if err != nil {
			continue
		}
		if python == "py" {
			return resolved, []string{"-3.13", "-m", "gallery_dl"}, nil
		}
		return resolved, []string{"-m", "gallery_dl"}, nil
	}

	return "", nil, fmt.Errorf("gallery-dl binary not found (install it with 'python -m pip install --user gallery-dl')")
}
