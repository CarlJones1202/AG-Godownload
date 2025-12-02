package main

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

func main() {
	// Initialize database
	database.Connect("../../gallery.db")

	fmt.Println("Starting color extraction for all images...")

	// Query images with their gallery/source information
	var images []struct {
		ID         uint
		Filename   string
		SourceName string
	}

	if err := database.DB.
		Table("images").
		Select("images.id, images.filename, COALESCE(sources.name, 'uncategorized') as source_name").
		Joins("LEFT JOIN image_galleries ON image_galleries.image_id = images.id").
		Joins("LEFT JOIN galleries ON galleries.id = image_galleries.gallery_id").
		Joins("LEFT JOIN sources ON sources.id = galleries.source_id").
		Where("images.dominant_colors IS NULL OR images.dominant_colors = ''").
		Group("images.id").
		Find(&images).Error; err != nil {
		fmt.Printf("Failed to query images: %v\n", err)
		os.Exit(1)
	}

	total := len(images)
	fmt.Printf("Found %d images without color data\n", total)

	if total == 0 {
		fmt.Println("All images already have color data!")
		return
	}

	var processed int32
	var failed int32
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8) // Process 8 images concurrently

	for _, img := range images {
		wg.Add(1)
		go func(id uint, filename string, sourceName string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Construct full path - check if filename already includes path
			var fullPath string
			if filepath.Dir(filename) != "." {
				// Filename already includes directory
				fullPath = filepath.Join("../../uploads", filename)
			} else {
				// Need to add source directory
				sanitizedSource := services.SanitizeDirectoryName(sourceName)
				fullPath = filepath.Join("../../uploads", sanitizedSource, filename)
			}

			// Check if file exists
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				// Try without source directory as fallback
				altPath := filepath.Join("../../uploads", filename)
				if _, err := os.Stat(altPath); os.IsNotExist(err) {
					fmt.Printf("[ID %d] File not found: %s (tried: %s and %s)\n", id, filename, fullPath, altPath)
					atomic.AddInt32(&failed, 1)
					return
				}
				fullPath = altPath
			}

			// Extract colors
			colors, err := services.ExtractDominantColors(fullPath)
			if err != nil {
				fmt.Printf("[ID %d] Failed to extract colors: %v\n", id, err)
				atomic.AddInt32(&failed, 1)
				return
			}

			// Update database
			if err := database.DB.Model(&models.Image{ID: id}).Update("dominant_colors", colors).Error; err != nil {
				fmt.Printf("[ID %d] Failed to update database: %v\n", id, err)
				atomic.AddInt32(&failed, 1)
				return
			}

			current := atomic.AddInt32(&processed, 1)
			if current%100 == 0 {
				fmt.Printf("Progress: %d/%d (%.1f%%)\n", current, total, float64(current)/float64(total)*100)
			}
		}(img.ID, img.Filename, img.SourceName)
	}

	wg.Wait()

	fmt.Printf("\n=== Color Extraction Complete ===\n")
	fmt.Printf("Total: %d\n", total)
	fmt.Printf("Processed: %d\n", atomic.LoadInt32(&processed))
	fmt.Printf("Failed: %d\n", atomic.LoadInt32(&failed))
}
