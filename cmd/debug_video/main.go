package main

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"log"
	"os"
	"path/filepath"
)

func main() {
	database.Connect("gallery.db")
	services.UploadsDir = "uploads" // Ensure this is set

	var video models.Image
	// Find a video with missing metadata
	if err := database.DB.Where("type = ? AND duration = ?", "video", 0).First(&video).Error; err != nil {
		log.Fatalf("No broken videos found: %v", err)
	}

	fmt.Printf("DEBUG: Found Broken Video ID: %d\n", video.ID)
	fmt.Printf("DEBUG: Filename: %s\n", video.Filename)
	fmt.Printf("DEBUG: SourceID: %v\n", video.SourceID)

	// Try to find file using recursive global search
	foundPath := ""
	baseName := filepath.Base(video.Filename)
	fmt.Printf("DEBUG: Searching for basename: %s\n", baseName)

	err := filepath.Walk("uploads", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && filepath.Base(path) == baseName {
			foundPath = path
			return nil
		}
		return nil
	})

	if foundPath == "" {
		fmt.Printf("FATAL: File not found recursively in uploads!\n")
		return
	}

	fmt.Printf("DEBUG: Found file at: %s\n", foundPath)

	// Run ffprobe manually via service
	meta, err := services.GetVideoMetadata(foundPath)
	if err != nil {
		fmt.Printf("FATAL: GetVideoMetadata failed: %v\n", err)
		return
	}

	fmt.Printf("SUCCESS: Metadata extracted! Duration: %f, Size: %f\n", meta.Duration, meta.SizeMB)
}
