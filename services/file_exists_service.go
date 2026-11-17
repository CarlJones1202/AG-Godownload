package services

import (
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// statConcurrency bounds parallel os.Stat calls during the disk-state sweep.
const statConcurrency = 64

// uploadsDirHasEntries reports whether UploadsDir contains any directory
// entries. A missing directory means no files are on disk (fresh install), so
// the caller can skip the per-row stat sweep. Any other read error is treated
// as "entries present" so the full scan runs rather than assuming emptiness.
func uploadsDirHasEntries() bool {
	entries, err := os.ReadDir(UploadsDir)
	if err != nil {
		return !os.IsNotExist(err)
	}
	return len(entries) > 0
}

// SyncFileExists scans all images in the database in batches and updates their file_exists column.
func SyncFileExists() error {
	logger.Info("Starting file_exists synchronization...")

	// Fresh-install fast path: with no files on disk every row is missing by
	// definition, so one UPDATE replaces the O(rows) stat sweep.
	if !uploadsDirHasEntries() {
		res := database.DB.Model(&models.Image{}).
			Where("file_exists = ?", true).
			Update("file_exists", false)
		if res.Error != nil {
			logger.Errorf("Fresh-install file_exists update failed: %v", res.Error)
			return res.Error
		}
		logger.Infof("File exists sync (fresh install, no files on disk): %d rows marked missing, stat sweep skipped", res.RowsAffected)
		return nil
	}

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

		// Stat calls are latency-bound I/O; run them concurrently within the
		// page (bounded by statConcurrency) instead of one at a time.
		exists := make([]bool, len(page))
		var wg sync.WaitGroup
		sem := make(chan struct{}, statConcurrency)
		for i := range page {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				trimmed := strings.TrimSpace(page[i].Filename)
				if trimmed == "" {
					return // exists stays false for rows without a filename
				}
				_, err := os.Stat(filepath.Join(UploadsDir, trimmed))
				exists[i] = err == nil
			}(i)
		}
		wg.Wait()

		var toTrueIDs []uint
		var toFalseIDs []uint

		for i, img := range page {
			totalScanned++
			if exists[i] {
				onDiskCount++
			}

			if exists[i] != img.FileExists {
				if exists[i] {
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
