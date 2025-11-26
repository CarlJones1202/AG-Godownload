package services

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// VerifyDownloadedImages checks all images in the database and re-downloads any missing files
// VerifyDownloadedImages checks all images and re-downloads + fixes DB if needed
func VerifyDownloadedImages() error {
	logger.Info("Verifying downloaded images...")

	var images []struct {
		ID          uint
		Filename    string
		DownloadURL string
	}

	if err := database.DB.
		Model(&models.Image{}).
		Select("id, filename, download_url").
		Find(&images).Error; err != nil {
		return fmt.Errorf("failed to query images: %w", err)
	}

	// Run the actual verification in background (as you intended)
	go func() {
		missingCount := 0
		var redownloadedCount int32 = 0

		type redownloadTask struct {
			ID          uint
			OldFilename string
			DownloadURL string
			SourceName  string
		}

		var redownloadTasks []redownloadTask
		var mu sync.Mutex

		for _, img := range images {
			currentPath := filepath.Join(UploadsDir, img.Filename)

			fileMissing := false
			if _, err := os.Stat(currentPath); os.IsNotExist(err) {
				fmt.Printf("Missing image file: %s (ID: %d)\n", img.Filename, img.ID)
				missingCount++
				fileMissing = true
			}

			// Thumbnail regen logic (unchanged)
			thumbPath := filepath.Join(UploadsDir, "thumbnails", img.Filename)
			if _, err := os.Stat(thumbPath); os.IsNotExist(err) && !fileMissing {
				if _, err := os.Stat(currentPath); err == nil {
					fmt.Printf("Regenerating missing thumbnail for: %s\n", img.Filename)
					GenerateThumbnail(currentPath)
				}
			}

			if fileMissing && img.DownloadURL != "" {
				sourceName := "uncategorized"

				var sourceID *uint
				err := database.DB.
					Table("galleries").
					Select("galleries.source_id").
					Joins("JOIN image_galleries ON image_galleries.gallery_id = galleries.id").
					Where("image_galleries.image_id = ?", img.ID).
					Limit(1).
					Scan(&sourceID).Error

				if err == nil && sourceID != nil {
					var source models.Source
					if database.DB.Select("name").First(&source, *sourceID).Error == nil {
						sourceName = source.Name
					}
				}

				fmt.Printf("Re-downloading from: %s (Source: %s)\n", img.DownloadURL, sourceName)

				mu.Lock()
				redownloadTasks = append(redownloadTasks, redownloadTask{
					ID:          img.ID,
					OldFilename: img.Filename,
					DownloadURL: img.DownloadURL,
					SourceName:  sourceName,
				})
				mu.Unlock()
			} else if fileMissing {
				fmt.Printf("No download URL available for %s, cannot recover\n", img.Filename)
			}
		}

		// === CONCURRENT REDOWNLOAD + DB FIX ===
		if len(redownloadTasks) > 0 {
			fmt.Printf("Starting concurrent re-download of %d missing images (max 12 at a time)...\n", len(redownloadTasks))

			const maxConcurrent = 12
			sem := make(chan struct{}, maxConcurrent)
			var wg sync.WaitGroup

			for _, task := range redownloadTasks {
				wg.Add(1)
				go func(t redownloadTask) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()

					start := time.Now()
					newFullPath, err := DownloadImage(t.DownloadURL, t.SourceName)
					duration := time.Since(start)

					if duration > 5*time.Second {
						time.Sleep(1 * time.Second)
					}

					if err != nil {
						fmt.Printf("Failed to re-download image ID %d: %v\n", t.ID, err)
						return
					}

					// === THIS IS THE CRITICAL FIX ===
					newFilename := filepath.Base(newFullPath)
					expectedPath := filepath.Join(UploadsDir, t.SourceName, newFilename)

					// If the returned path is different from expected, fix it
					if newFullPath != expectedPath {
						os.Rename(newFullPath, expectedPath)
						newFullPath = expectedPath
					}

					// Extract relative path for DB: "MySource/abc123.jpg"

					// Update DB only if filename changed
					if newFilename != t.OldFilename {
						result := database.DB.Model(&models.Image{ID: t.ID}).Update("filename", newFilename)
						if result.Error != nil {
							fmt.Printf("Failed to update filename in DB for image %d: %v\n", t.ID, result.Error)
						} else {
							fmt.Printf("Updated DB: %s → %s\n", t.OldFilename, newFilename)
						}
					}

					// Generate thumbnail
					if _, err = GenerateThumbnail(newFullPath); err != nil {
						fmt.Printf("Thumbnail generation failed for %s: %v\n", newFilename, err)
					} else {
						fmt.Printf("Successfully re-downloaded: %s (took %.2fs)\n", newFilename, duration.Seconds())
					}

					atomic.AddInt32(&redownloadedCount, 1)
				}(task)
			}

			wg.Wait()
		}

		fmt.Printf("Verification complete. Found %d missing images, successfully re-downloaded %d.\n",
			missingCount, atomic.LoadInt32(&redownloadedCount))
	}()

	return nil
}
