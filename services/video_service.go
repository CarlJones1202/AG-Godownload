package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ImportLocalVideo imports a video file from a local path
func ImportLocalVideo(sourcePath string, sourceName string) (*DownloadImageResult, error) {
	// Verify source file exists
	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("source is a directory, not a file")
	}

	// Read file for hashing
	f, err := os.Open(sourcePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return nil, err
	}
	hashStr := hex.EncodeToString(hash.Sum(nil))

	// Get extension
	ext := filepath.Ext(sourcePath)
	filename := hashStr + ext

	// Setup destination
	sourceDir := SanitizeDirectoryName(sourceName)
	if sourceDir == "" {
		sourceDir = "unknown"
	}

	fullDir := filepath.Join(UploadsDir, sourceDir)
	if err := os.MkdirAll(fullDir, 0755); err != nil {
		return nil, err
	}

	destPath := filepath.Join(fullDir, filename)

	// Check collision
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		// Close the reader first
		f.Close()

		if err := os.Rename(sourcePath, destPath); err != nil {
			// Fallback to copy
			if err := copyFile(sourcePath, destPath); err != nil {
				return nil, err
			}
			// Delete original if copy successful
			os.Remove(sourcePath)
		}
	} else {
		// If file exists, we still might want to delete the source if we are "moving" it
		f.Close()
		os.Remove(sourcePath)
	}

	// Generate thumbnail
	if _, err := GenerateVideoThumbnail(destPath); err != nil {
		fmt.Printf("Warning: Failed to generate video thumbnail: %v\n", err)
	}

	// Generate trickplay data (sprite sheet + VTT file)
	if err := GenerateTrickplayData(destPath); err != nil {
		fmt.Printf("Warning: Failed to generate trickplay data: %v\n", err)
	}

	// Parse Video Metadata
	meta, err := GetVideoMetadata(destPath)
	if err != nil {
		fmt.Printf("Warning: Failed to get video metadata: %v\n", err)
		meta = &VideoMetadata{}
	}

	return &DownloadImageResult{
		Path:           destPath,
		Title:          CleanTitle(sourcePath), // Use cleaned filename as title
		Duration:       meta.Duration,
		Width:          meta.Width,
		Height:         meta.Height,
		SizeMB:         meta.SizeMB,
		DominantColors: "[]",
	}, nil
}

// Helper to clean filename for title
func CleanTitle(filename string) string {
	name := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	// Replace underscores and dots with spaces
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, ".", " ")
	// Also handle dashes
	name = strings.ReplaceAll(name, "-", " ")
	return strings.TrimSpace(name)
}

// IsVideoFile checks if extension matches common video formats
func IsVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp4", ".mkv", ".webm", ".avi", ".mov", ".wmv", ".flv":
		return true
	}
	return false
}

// IsVideoURL checks if a URL points to a video based on extension or domain
func IsVideoURL(url string) bool {
	// Check extension
	if IsVideoFile(url) {
		return true
	}
	// Check known video sites
	if strings.Contains(url, "tnaflix.com") {
		return true
	}
	if strings.Contains(url, "pornhub.com") {
		return true
	}
	if strings.Contains(url, "pmvhaven.com") {
		return true
	}
	if strings.Contains(url, "youtube.com") || strings.Contains(url, "youtu.be") {
		return true
	}
	return false
}
