package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/disintegration/imaging"
)

const (
	UploadsDir = "uploads"
)

func EnsureUploadsDir() error {
	if _, err := os.Stat(UploadsDir); os.IsNotExist(err) {
		return os.MkdirAll(UploadsDir, 0755)
	}
	return nil
}

// SanitizeDirectoryName removes invalid characters from directory names
func SanitizeDirectoryName(name string) string {
	// Replace invalid characters with underscores
	reg := regexp.MustCompile(`[<>:"/\\|?*]`)
	sanitized := reg.ReplaceAllString(name, "_")
	// Remove leading/trailing spaces and dots
	sanitized = strings.Trim(sanitized, " .")
	// Limit length
	if len(sanitized) > 100 {
		sanitized = sanitized[:100]
	}
	return sanitized
}

// DownloadImage downloads an image and saves it with a content-based hash filename
// in a subdirectory named after the source
func DownloadImage(url string, sourceName string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download image: status code %d", resp.StatusCode)
	}

	// Read the entire response body into memory
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Calculate SHA-256 hash of the content
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	// Determine extension from content type
	ext := ".jpg" // default
	contentType := resp.Header.Get("Content-Type")
	switch contentType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	}

	// Create filename from hash
	filename := hashStr + ext

	// Sanitize source name for directory
	sourceDir := SanitizeDirectoryName(sourceName)
	if sourceDir == "" {
		sourceDir = "unknown"
	}

	// Create source subdirectory
	fullDir := filepath.Join(UploadsDir, sourceDir)
	if err := os.MkdirAll(fullDir, 0755); err != nil {
		return "", err
	}

	// Full path for the image
	destPath := filepath.Join(fullDir, filename)

	// Check if file already exists (same content hash)
	if _, err := os.Stat(destPath); err == nil {
		// File already exists, return the path
		return destPath, nil
	}

	// Write the file
	out, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = out.Write(data)
	if err != nil {
		return "", err
	}

	return destPath, nil
}

func GenerateThumbnail(srcPath string) (string, error) {
	src, err := imaging.Open(srcPath)
	if err != nil {
		return "", err
	}

	// Resize to width 200 using Lanczos filter.
	dst := imaging.Resize(src, 200, 0, imaging.Lanczos)

	// Get the directory structure from source path
	// srcPath is like "uploads/source_name/hash.jpg"
	// We want "uploads/source_name/thumbnails/hash.jpg"
	dir := filepath.Dir(srcPath)
	thumbnailDir := filepath.Join(dir, "thumbnails")

	if err := os.MkdirAll(thumbnailDir, 0755); err != nil {
		return "", err
	}

	// Use same filename as original, but in thumbnails subdirectory
	filename := filepath.Base(srcPath)
	thumbPath := filepath.Join(thumbnailDir, filename)

	// Save the resulting image as JPEG.
	err = imaging.Save(dst, thumbPath, imaging.JPEGQuality(80))
	if err != nil {
		return "", err
	}

	return thumbPath, nil
}
