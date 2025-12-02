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

	// FIX: Load only what we need — NO Preload("Galleries") → avoids SQLite variable explosion
	var images []struct {
		ID          uint
		Filename    string
		DownloadURL string
	}
	if err := database.DB.
		Model(&models.Image{}).
		Select("id, filename, download_url").
		Find(&images).Error; err != nil {
		return fmt.Errorf("failed to load images: %w", err)
	}

	logger.Infof("Found %d images to migrate", len(images))

	migrated := 0
	redownloaded := 0
	skipped := 0
	errors := 0

	for _, image := range images {
		// Lazily determine source name — only when needed, and safely
		sourceName := "unknown"

		var sourceID *uint
		err := database.DB.
			Table("galleries").
			Select("galleries.source_id").
			Joins("JOIN image_galleries ON image_galleries.gallery_id = galleries.id").
			Where("image_galleries.image_id = ?", image.ID).
			Limit(1).
			Scan(&sourceID).Error

		if err == nil && sourceID != nil {
			var source models.Source
			if err := database.DB.Select("name").First(&source, *sourceID).Error; err == nil {
				sourceName = source.Name
			} else {
				sourceName = "uncategorized"
			}
		} else {
			sourceName = "uncategorized"
		}

		oldPath := filepath.Join(UploadsDir, image.Filename)
		oldThumbPath := filepath.Join(UploadsDir, "thumbnails", image.Filename)

		// Check if file exists in old location
		if _, err := os.Stat(oldPath); os.IsNotExist(err) {
			// Check if already in new location
			sourceDir := SanitizeDirectoryName(sourceName)
			newPath := filepath.Join(UploadsDir, sourceDir, image.Filename)

			if _, err := os.Stat(newPath); err == nil {
				skipped++
				continue
			}

			// File is truly missing → re-download
			logger.Warnf("Image file missing for %s, re-downloading from %s", image.Filename, image.DownloadURL)

			if image.DownloadURL == "" {
				logger.Errorf("Cannot re-download %s: no download URL", image.Filename)
				errors++
				continue
			}

			result, err := DownloadImage(image.DownloadURL, sourceName)
			if err != nil {
				logger.Errorf("Failed to re-download %s: %v", image.Filename, err)
				errors++
				continue
			}

			_, err = GenerateThumbnail(result.Path)
			if err != nil {
				logger.Warnf("Failed to generate thumbnail for %s: %v", image.Filename, err)
			}

			newFilename := filepath.Base(result.Path)
			if err := database.DB.Model(&models.Image{ID: image.ID}).Updates(map[string]interface{}{
				"filename":        newFilename,
				"dominant_colors": result.DominantColors,
			}).Error; err != nil {
				logger.Errorf("Failed to update filename in database: %v", err)
				errors++
				continue
			}

			redownloaded++
			logger.Debugf("Re-downloaded %s → %s", image.Filename, newFilename)
			continue
		}

		// File exists in old location → migrate it
		data, err := os.ReadFile(oldPath)
		if err != nil {
			logger.Errorf("Failed to read %s: %v", oldPath, err)
			errors++
			continue
		}

		hash := sha256.Sum256(data)
		hashStr := hex.EncodeToString(hash[:])
		ext := filepath.Ext(image.Filename)
		if ext == "" {
			ext = ".jpg"
		}
		newFilename := hashStr + ext

		sourceDir := SanitizeDirectoryName(sourceName)
		newDir := filepath.Join(UploadsDir, sourceDir)
		if err := os.MkdirAll(newDir, 0755); err != nil {
			logger.Errorf("Failed to create directory %s: %v", newDir, err)
			errors++
			continue
		}

		newPath := filepath.Join(newDir, newFilename)

		// Handle hash collision
		if _, err := os.Stat(newPath); err == nil {
			logger.Infof("Hash collision detected for %s, re-downloading to ensure separate copy", image.Filename)

			if image.DownloadURL != "" {
				result, err := DownloadImage(image.DownloadURL, sourceName)
				if err != nil {
					logger.Errorf("Failed to re-download %s: %v", image.Filename, err)
					// Fall back: just point to existing file
					database.DB.Model(&models.Image{ID: image.ID}).Update("filename", filepath.Join(sourceDir, newFilename))
				} else {
					_, err = GenerateThumbnail(result.Path)
					if err != nil {
						logger.Warnf("Failed to generate thumbnail: %v", err)
					}
					freshFilename := filepath.Base(result.Path)
					database.DB.Model(&models.Image{ID: image.ID}).Updates(map[string]interface{}{
						"filename":        freshFilename,
						"dominant_colors": result.DominantColors,
					})
					redownloaded++
				}
			} else {
				database.DB.Model(&models.Image{ID: image.ID}).Update("filename", filepath.Join(sourceDir, newFilename))
			}

			os.Remove(oldPath)
			os.Remove(oldThumbPath)
			skipped++
			continue
		}

		// Actually move the file
		if err := os.Rename(oldPath, newPath); err != nil {
			logger.Errorf("Failed to move %s to %s: %v", oldPath, newPath, err)
			errors++
			continue
		}

		// Move thumbnail
		newThumbDir := filepath.Join(newDir, "thumbnails")
		os.MkdirAll(newThumbDir, 0755)
		newThumbPath := filepath.Join(newThumbDir, newFilename)
		if _, err := os.Stat(oldThumbPath); err == nil {
			os.Rename(oldThumbPath, newThumbPath)
		}

		// Update DB
		fullNewFilename := filepath.Join(sourceDir, newFilename)
		if err := database.DB.Model(&models.Image{ID: image.ID}).Update("filename", fullNewFilename).Error; err != nil {
			logger.Errorf("Failed to update filename in database: %v", err)
			errors++
			continue
		}

		migrated++
		logger.Debugf("Migrated %s → %s", image.Filename, fullNewFilename)
	}

	logger.Infof("Migration complete: %d migrated, %d re-downloaded, %d skipped, %d errors",
		migrated, redownloaded, skipped, errors)

	return nil
}
