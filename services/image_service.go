package services

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

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

func DownloadImage(url string, filename string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download image: status code %d", resp.StatusCode)
	}

	// Determine extension if not present in filename
	ext := filepath.Ext(filename)
	if ext == "" {
		contentType := resp.Header.Get("Content-Type")
		switch contentType {
		case "image/jpeg":
			ext = ".jpg"
		case "image/png":
			ext = ".png"
		case "image/gif":
			ext = ".gif"
		default:
			// Default to jpg if unknown
			ext = ".jpg"
		}
		filename = filename + ext
	}

	destPath := filepath.Join(UploadsDir, filename)
	out, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	_, err = out.ReadFrom(resp.Body)
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

	// Create thumbnails subdirectory
	thumbnailDir := filepath.Join(UploadsDir, "thumbnails")
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
