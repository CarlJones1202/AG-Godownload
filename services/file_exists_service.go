package services

import (
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"os"
	"path/filepath"
)

// SyncFileExists scans all images in the database in batches and updates their file_exists column
func SyncFileExists() error {
	logger.Info("Starting file_exists synchronization...")

	const pageSize = 500
	var lastID uint
	totalScanned := 0
	onDiskCount := 0
	updatedCount := 0

	for {
		type ImageCheck struct {
			ID         uint
			Filename   string
			FileExists bool
		}
		var page []ImageCheck

		if err := database.DB.Model(&models.Image{}).
			Select("id, filename, file_exists").
			Where("id > ?", lastID).
			Order("id ASC").
			Limit(pageSize).
			Find(&page).Error; err != nil {
			logger.Errorf("SyncFileExists query error: %v", err)
			return err
		}

		if len(page) == 0 {
			break
		}

		var toTrueIDs []uint
		var toFalseIDs []uint

		for _, img := range page {
			totalScanned++
			fullPath := filepath.Join(UploadsDir, img.Filename)
			_, err := os.Stat(fullPath)
			existsOnDisk := (err == nil)

			if existsOnDisk {
				onDiskCount++
			}

			if existsOnDisk != img.FileExists {
				if existsOnDisk {
					toTrueIDs = append(toTrueIDs, img.ID)
				} else {
					toFalseIDs = append(toFalseIDs, img.ID)
				}
			}
		}

		if len(toTrueIDs) > 0 {
			if err := database.DB.Model(&models.Image{}).
				Where("id IN ?", toTrueIDs).
				Update("file_exists", true).Error; err != nil {
				logger.Errorf("Failed to update file_exists=true batch: %v", err)
			}
			updatedCount += len(toTrueIDs)
		}

		if len(toFalseIDs) > 0 {
			if err := database.DB.Model(&models.Image{}).
				Where("id IN ?", toFalseIDs).
				Update("file_exists", false).Error; err != nil {
				logger.Errorf("Failed to update file_exists=false batch: %v", err)
			}
			updatedCount += len(toFalseIDs)
		}

		lastID = page[len(page)-1].ID
	}

	logger.Infof("File exists sync completed: %d total scanned, %d on disk, %d updated in DB", totalScanned, onDiskCount, updatedCount)
	return nil
}
