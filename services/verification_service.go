package services

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"os"
	"path/filepath"
)

// VerifyDownloadedImages checks all images in the database and re-downloads any missing files
func VerifyDownloadedImages() error {
	logger.Info("Verifying downloaded images...")

	var images []models.Image
	if err := database.DB.Preload("Galleries").Find(&images).Error; err != nil {
		return err
	}

	missingCount := 0
	redownloadedCount := 0

	for _, image := range images {
		// Check if the image file exists
		imagePath := filepath.Join(UploadsDir, image.Filename)
		if _, err := os.Stat(imagePath); os.IsNotExist(err) {
			fmt.Printf("Missing image file: %s (ID: %d)\n", image.Filename, image.ID)
			missingCount++

			// Try to re-download if we have a download URL
			if image.DownloadURL != "" {
				// Determine source name
				var sourceName string
				if len(image.Galleries) > 0 {
					gallery := image.Galleries[0]
					if gallery.SourceID != nil {
						var source models.Source
						if err := database.DB.First(&source, *gallery.SourceID).Error; err == nil {
							sourceName = source.Name
						} else {
							sourceName = "uncategorized"
						}
					} else {
						sourceName = "uncategorized"
					}
				} else {
					sourceName = "uncategorized"
				}

				fmt.Printf("Re-downloading from: %s (Source: %s)\n", image.DownloadURL, sourceName)
				destPath, err := DownloadImage(image.DownloadURL, sourceName)
				if err != nil {
					fmt.Printf("Failed to re-download %s: %v\n", image.Filename, err)
					continue
				}

				// Regenerate thumbnail
				_, err = GenerateThumbnail(destPath)
				if err != nil {
					fmt.Printf("Failed to regenerate thumbnail for %s: %v\n", image.Filename, err)
				}

				redownloadedCount++
				fmt.Printf("Successfully re-downloaded: %s\n", image.Filename)
			} else {
				fmt.Printf("No download URL available for %s, skipping\n", image.Filename)
			}
		}

		// Also check thumbnail
		thumbnailPath := filepath.Join(UploadsDir, "thumbnails", image.Filename)
		if _, err := os.Stat(thumbnailPath); os.IsNotExist(err) {
			// Try to regenerate thumbnail if original exists
			if _, err := os.Stat(imagePath); err == nil {
				fmt.Printf("Regenerating missing thumbnail for: %s\n", image.Filename)
				_, err = GenerateThumbnail(imagePath)
				if err != nil {
					fmt.Printf("Failed to regenerate thumbnail for %s: %v\n", image.Filename, err)
				}
			}
		}
	}

	fmt.Printf("Verification complete. Found %d missing images, re-downloaded %d\n", missingCount, redownloadedCount)
	return nil
}
