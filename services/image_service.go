package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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

// DownloadImageResult contains the result of downloading an image
type DownloadImageResult struct {
	Path           string
	DominantColors string
}

// DownloadImage downloads an image and saves it with a content-based hash filename
// in a subdirectory named after the source. Returns the path and extracted colors.
func DownloadImage(url string, sourceName string) (*DownloadImageResult, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download image: status code %d", resp.StatusCode)
	}

	// Read the entire response body into memory
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	// Full path for the image
	destPath := filepath.Join(fullDir, filename)

	// Check if file already exists (same content hash)
	fileExists := false
	if _, err := os.Stat(destPath); err == nil {
		fileExists = true
	}

	if !fileExists {
		// Write the file
		out, err := os.Create(destPath)
		if err != nil {
			return nil, err
		}
		defer out.Close()

		_, err = out.Write(data)
		if err != nil {
			return nil, err
		}
	}

	// Extract dominant colors
	colors, err := ExtractDominantColors(destPath)
	if err != nil {
		// Don't fail the whole operation if color extraction fails
		colors = "[]"
	}

	return &DownloadImageResult{
		Path:           destPath,
		DominantColors: colors,
	}, nil
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

// GenerateVideoThumbnail generates a thumbnail for a video file using ffmpeg
func GenerateVideoThumbnail(srcPath string) (string, error) {
	// Get the directory structure from source path
	dir := filepath.Dir(srcPath)
	thumbnailDir := filepath.Join(dir, "thumbnails")

	if err := os.MkdirAll(thumbnailDir, 0755); err != nil {
		return "", err
	}

	// Use same filename as original + .jpg
	filename := filepath.Base(srcPath) + ".jpg"
	thumbPath := filepath.Join(thumbnailDir, filename)

	// Check if thumbnail already exists
	if _, err := os.Stat(thumbPath); err == nil {
		return thumbPath, nil
	}

	// Use ffmpeg to extract a frame at 5 seconds or 10%?
	// Let's try 00:00:01 for now to likely hit content but not black start
	// -vframes 1: output one frame
	// -ss 1: seek to 1 second
	cmd := exec.Command("ffmpeg", "-y", "-i", srcPath, "-ss", "00:00:01", "-vframes", "1", thumbPath)

	// Capture output in case of error
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg failed: %w, output: %s", err, string(output))
	}

	return thumbPath, nil
}

// DownloadPersonImage downloads an image for a person and saves it to a specific directory
func DownloadPersonImage(url string, personID uint) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download image: status code %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Calculate hash for filename
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	// Determine extension
	ext := ".jpg"
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

	filename := hashStr + ext

	// Create person-specific directory: uploads/person_images/{id}
	personDir := filepath.Join(UploadsDir, "person_images", fmt.Sprintf("%d", personID))
	if err := os.MkdirAll(personDir, 0755); err != nil {
		return "", err
	}

	destPath := filepath.Join(personDir, filename)

	// Write file if it doesn't exist
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return "", err
		}
	}

	// Return web-accessible path
	return fmt.Sprintf("/person-images/%d/%s", personID, filename), nil
}
