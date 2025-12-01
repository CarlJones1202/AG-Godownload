package services

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// VerifyDownloadedImages checks all images in the database and re-downloads any missing files
// VerifyDownloadedImages checks all images and re-downloads + fixes DB if needed
func VerifyDownloadedImages() error {
	logger.Info("Verifying downloaded images...")

	var images []struct {
		ID          uint
		Filename    string // current value in DB (may point to missing/broken file)
		DownloadURL string
	}

	if err := database.DB.
		Model(&models.Image{}).
		Select("id, filename, download_url").
		Find(&images).Error; err != nil {
		return fmt.Errorf("failed to query images: %w", err)
	}

	go func() {
		missingCount := 0
		var recoveredCount int32 = 0

		type recoveryTask struct {
			ID            uint
			CurrentDBPath string // what DB currently thinks the path is
			DownloadURL   string
			SourceName    string
		}

		var tasks []recoveryTask
		var mu sync.Mutex

		// Phase 1: scan and collect missing files
		for _, img := range images {
			expectedFullPath := filepath.Join(UploadsDir, img.Filename)

			if _, err := os.Stat(expectedFullPath); os.IsNotExist(err) {
				missingCount++
				fmt.Printf("Missing: %s (ID: %d)\n", img.Filename, img.ID)

				// Optional: regenerate thumbnail if main file exists but thumb doesn't
				thumbPath := filepath.Join(UploadsDir, "thumbnails", filepath.Base(img.Filename))
				if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
					if _, err := os.Stat(expectedFullPath); err == nil {
						go GenerateThumbnail(expectedFullPath)
					}
				}

				if img.DownloadURL == "" {
					fmt.Printf("No DownloadURL for ID %d → cannot recover\n", img.ID)
					continue
				}

				// Resolve source name (same logic you already had)
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
					var src models.Source
					if database.DB.Select("name").First(&src, *sourceID).Error == nil {
						sourceName = src.Name
					}
				}

				mu.Lock()
				tasks = append(tasks, recoveryTask{
					ID:            img.ID,
					CurrentDBPath: img.Filename,
					DownloadURL:   img.DownloadURL,
					SourceName:    sourceName,
				})
				mu.Unlock()
			}
		}

		// Phase 2: concurrent recovery – THE IMPORTANT PART
		if len(tasks) > 0 {
			fmt.Printf("Recovering %d missing images (max 12 concurrent)...\n", len(tasks))

			const maxConcurrent = 12
			sem := make(chan struct{}, maxConcurrent)
			var wg sync.WaitGroup

			for _, task := range tasks {
				wg.Add(1)
				go func(t recoveryTask) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()

					start := time.Now()

					// This returns the FINAL path where it saved the file
					// (e.g. /uploads/MySource/abc123.jpg or whatever it decided)
					finalPathFromDownloader, err := DownloadImage(t.DownloadURL, t.SourceName)
					if err != nil {
						fmt.Printf("[ID %d] Re-download failed: %v\n", t.ID, err)
						return
					}

					// --- CRITICAL FIX: ALWAYS force a unique filename in the correct source dir ---
					ext := filepath.Ext(finalPathFromDownloader)
					newFilename := uuid.New().String() + ext
					correctDir := filepath.Join(UploadsDir, t.SourceName)
					correctFinalPath := filepath.Join(correctDir, newFilename)

					os.MkdirAll(correctDir, 0o755)

					// If DownloadImage already put it in the right place with a UUID → keep it
					// Otherwise → move + rename to guaranteed-unique name
					if finalPathFromDownloader != correctFinalPath {
						if err := os.Rename(finalPathFromDownloader, correctFinalPath); err != nil {
							// If rename fails (e.g. cross-device), fall back to copy+delete
							if err := copyFile(finalPathFromDownloader, correctFinalPath); err != nil {
								fmt.Printf("[ID %d] Failed to move/copy recovered file: %v\n", t.ID, err)
								return
							}
							os.Remove(finalPathFromDownloader)
						}
					}

					// --- ALWAYS update DB to the new guaranteed-unique relative path ---
					newRelativePath := filepath.Join(t.SourceName, newFilename)

					if err := database.DB.
						Model(&models.Image{ID: t.ID}).
						Update("filename", newRelativePath).Error; err != nil {

						fmt.Printf("[ID %d] Failed to update DB filename: %v\n", t.ID, err)
					} else {
						fmt.Printf("[ID %d] Recovered → %s\n", t.ID, newRelativePath)
					}

					// Generate thumbnail
					if _, err := GenerateThumbnail(correctFinalPath); err != nil {
						fmt.Printf("[ID %d] Thumbnail generation failed: %v\n", t.ID, err)
					}

					duration := time.Since(start)
					if duration > 5*time.Second {
						time.Sleep(1 * time.Second) // polite pause
					}

					atomic.AddInt32(&recoveredCount, 1)
				}(task)
			}

			wg.Wait()
		}

		fmt.Printf("Verification complete — Missing: %d | Recovered: %d\n",
			missingCount, atomic.LoadInt32(&recoveredCount))
	}()

	return nil
}

// Helper for cross-device moves
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}
