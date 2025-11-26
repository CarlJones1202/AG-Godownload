package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"os"
	"path/filepath"
)

// MigrateImagesToNewStructure migrates existing images from flat structure to source-based subdirectories
func MigrateImagesToNewStructure() error {
	logger.Info("Starting image migration to new directory structure...")

	var images []models.Image
	if err := database.DB.Preload("Galleries").Find(&images).Error; err != nil {
		return fmt.Errorf("failed to load images: %v", err)
	}

	logger.Infof("Found %d images to migrate", len(images))

	migrated := 0
	redownloaded := 0
	skipped := 0
	errors := 0

	for _, image := range images {
		// Determine source name from first gallery
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
			sourceName = "unknown"
		}

		oldPath := filepath.Join(UploadsDir, image.Filename)
		oldThumbPath := filepath.Join(UploadsDir, "thumbnails", image.Filename)

		// Check if file exists in old location
		if _, err := os.Stat(oldPath); os.IsNotExist(err) {
			// File doesn't exist in old location, might already be migrated or missing
			// Check if it exists in new location
			sourceDir := SanitizeDirectoryName(sourceName)
			newPath := filepath.Join(UploadsDir, sourceDir, image.Filename)

			if _, err := os.Stat(newPath); err == nil {
				// Already in new location
				skipped++
				continue
			}

			// File is missing, need to re-download
			logger.Warnf("Image file missing for %s, re-downloading from %s", image.Filename, image.DownloadURL)

			if image.DownloadURL == "" {
				logger.Errorf("Cannot re-download %s: no download URL", image.Filename)
				errors++
				continue
			}

			// Re-download the image
			newPath, err := DownloadImage(image.DownloadURL, sourceName)
			if err != nil {
				logger.Errorf("Failed to re-download %s: %v", image.Filename, err)
				errors++
				continue
			}

			// Generate thumbnail
			_, err = GenerateThumbnail(newPath)
			if err != nil {
				logger.Warnf("Failed to generate thumbnail for %s: %v", image.Filename, err)
			}

			// Update database with new filename
			newFilename := filepath.Base(newPath)
			if err := database.DB.Model(&image).Update("filename", newFilename).Error; err != nil {
				logger.Errorf("Failed to update filename in database: %v", err)
				errors++
				continue
			}

			redownloaded++
			logger.Debugf("Re-downloaded %s -> %s", image.Filename, newFilename)
			continue
		}

		// File exists in old location, migrate it
		// Read file content to calculate hash
		data, err := os.ReadFile(oldPath)
		if err != nil {
			logger.Errorf("Failed to read %s: %v", oldPath, err)
			errors++
			continue
		}

		// Calculate hash
		hash := sha256.Sum256(data)
		hashStr := hex.EncodeToString(hash[:])

		// Determine extension
		ext := filepath.Ext(image.Filename)
		if ext == "" {
			ext = ".jpg"
		}

		// New filename based on hash
		newFilename := hashStr + ext

		// Create source subdirectory
		sourceDir := SanitizeDirectoryName(sourceName)
		newDir := filepath.Join(UploadsDir, sourceDir)
		if err := os.MkdirAll(newDir, 0755); err != nil {
			logger.Errorf("Failed to create directory %s: %v", newDir, err)
			errors++
			continue
		}

		newPath := filepath.Join(newDir, newFilename)

		// Check if target already exists (collision)
		if _, err := os.Stat(newPath); err == nil {
			// File with same hash already exists - this is a duplicate
			// Need to re-download for this gallery to ensure both galleries have their own copy
			logger.Infof("Hash collision detected for %s, re-downloading to ensure separate copy", image.Filename)

			if image.DownloadURL == "" {
				logger.Errorf("Cannot re-download %s: no download URL", image.Filename)
				// Just update the filename to point to existing file
				if err := database.DB.Model(&image).Update("filename", newFilename).Error; err != nil {
					logger.Errorf("Failed to update filename in database: %v", err)
					errors++
				}
				// Remove old file
				os.Remove(oldPath)
				os.Remove(oldThumbPath)
				skipped++
				continue
			}

			// Re-download to get a fresh copy
			freshPath, err := DownloadImage(image.DownloadURL, sourceName)
			if err != nil {
				logger.Errorf("Failed to re-download %s: %v", image.Filename, err)
				// Fall back to using existing file
				if err := database.DB.Model(&image).Update("filename", newFilename).Error; err != nil {
					logger.Errorf("Failed to update filename in database: %v", err)
					errors++
				}
				os.Remove(oldPath)
				os.Remove(oldThumbPath)
				errors++
				continue
			}

			// Generate thumbnail
			_, err = GenerateThumbnail(freshPath)
			if err != nil {
				logger.Warnf("Failed to generate thumbnail for %s: %v", image.Filename, err)
			}

			// Update database
			freshFilename := filepath.Base(freshPath)
			if err := database.DB.Model(&image).Update("filename", freshFilename).Error; err != nil {
				logger.Errorf("Failed to update filename in database: %v", err)
				errors++
				continue
			}

			// Remove old file
			os.Remove(oldPath)
			os.Remove(oldThumbPath)

			redownloaded++
			logger.Debugf("Re-downloaded due to collision: %s -> %s", image.Filename, freshFilename)
			continue
		}

		// Move file to new location
		if err := os.Rename(oldPath, newPath); err != nil {
			logger.Errorf("Failed to move %s to %s: %v", oldPath, newPath, err)
			errors++
			continue
		}

		// Move thumbnail
		newThumbDir := filepath.Join(newDir, "thumbnails")
		if err := os.MkdirAll(newThumbDir, 0755); err != nil {
			logger.Warnf("Failed to create thumbnail directory: %v", err)
		} else {
			newThumbPath := filepath.Join(newThumbDir, newFilename)
			if _, err := os.Stat(oldThumbPath); err == nil {
				if err := os.Rename(oldThumbPath, newThumbPath); err != nil {
					logger.Warnf("Failed to move thumbnail: %v", err)
				}
			}
		}

		// Update database with new filename
		if err := database.DB.Model(&image).Update("filename", newFilename).Error; err != nil {
			logger.Errorf("Failed to update filename in database: %v", err)
			errors++
			continue
		}

		migrated++
		logger.Debugf("Migrated %s -> %s/%s", image.Filename, sourceDir, newFilename)
	}

	logger.Infof("Migration complete: %d migrated, %d re-downloaded, %d skipped, %d errors",
		migrated, redownloaded, skipped, errors)

	return nil
}
