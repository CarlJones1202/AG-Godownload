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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/disintegration/imaging"
)

// DownloadImageWithGalleryDL attempts to download the given URL using the
// gallery-dl external tool. It's intended as a fallback for hosts like imx.to
// where a normal HTTP client may return 503 or other bot-blocking behavior.
func DownloadImageWithGalleryDL(ctx context.Context, url string, sourceName string, timeout time.Duration) (*DownloadImageResult, error) {
	if !config.Global.GalleryDL.Enabled {
		return nil, errors.New("gallery-dl fallback disabled in config")
	}

	bin := config.Global.GalleryDL.BinaryPath
	if bin == "" {
		bin = "gallery-dl"
	}

	// Ensure binary exists (in PATH) by trying to look it up via exec.LookPath
	if _, err := exec.LookPath(bin); err != nil {
		return nil, fmt.Errorf("gallery-dl binary not found: %s (install via inti.ps1 or 'python -m pip install --user gallery-dl')", bin)
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

	cmd := exec.CommandContext(cmdCtx, bin, "--no-part", "--range", "1-1", url)
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

	// Extract colors (best effort)
	colors, cerr := ExtractDominantColors(destPath)
	if cerr != nil {
		colors = "[]"
	}

	// Generate thumbnail (fire-and-forget may be acceptable but we run it here)
	if _, terr := GenerateThumbnail(destPath); terr != nil {
		logger.Warnf("gallery-dl: failed to generate thumbnail for %s: %v", destPath, terr)
	}

	logger.Debugf("gallery-dl: downloaded %s -> %s (size=%d) in tempdir %s", url, destPath, bestSize, tmpDir)

	return &DownloadImageResult{
		Path:           destPath,
		DominantColors: colors,
	}, nil
}
