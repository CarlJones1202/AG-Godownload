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
		// Copy file (better than move to avoid deleting original if something goes wrong, though user said "move it")
		// User said: "just move it from the folder to our uploads folder"
		// Okay, I'll attempt a Move (Rename), if valid. If different drive, copy+delete.

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
		// But only if hash is identical (which it is, by definition of filename)
		f.Close()
		os.Remove(sourcePath)
	}

	// Generate thumbnail
	if _, err := GenerateVideoThumbnail(destPath); err != nil {
		// Log error but don't fail import?
		// Since we can't easily log here without importing logger (circular dependency risk if not careful, but services package is same)
		// actually same package, so it's fine.
		// But function signature returns error. Let's return error for now to be safe or ignore.
		// video_service is in package services.
		fmt.Printf("Warning: Failed to generate video thumbnail: %v\n", err)
	}

	// Generate trickplay data (sprite sheet + VTT file)
	if err := GenerateTrickplayData(destPath); err != nil {
		fmt.Printf("Warning: Failed to generate trickplay data: %v\n", err)
	}

	return &DownloadImageResult{
		Path:           destPath,
		DominantColors: "[]",
	}, nil
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
