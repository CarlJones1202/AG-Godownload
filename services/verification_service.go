package services

import (
	"encoding/json"
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"io"
	"os"
	"path/filepath"
	"strings"
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
		Filename    string // current value in DB (may point to missing/broken file)
		DownloadURL string
	}

	if err := database.DB.
		Model(&models.Image{}).
		Select("id, filename, download_url").
		Order("is_favorite DESC").
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

		// Channel for batched updates
		type updateResult struct {
			ID             uint
			RelPath        string
			DominantColors string
		}
		resultChan := make(chan updateResult, 100)
		var wgBatch sync.WaitGroup

		// Start batch processor
		wgBatch.Add(1)
		go func() {
			defer wgBatch.Done()
			var buffer []updateResult
			const batchSize = 30
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()

			flush := func() {
				if len(buffer) == 0 {
					return
				}
				tx := database.DB.Begin()
				for _, res := range buffer {
					if err := tx.Model(&models.Image{ID: res.ID}).
						Updates(map[string]interface{}{
							"filename":        res.RelPath,
							"dominant_colors": res.DominantColors,
						}).Error; err != nil {
						logger.Errorf("Failed to update image %d in batch: %v", res.ID, err)
					} else {
						atomic.AddInt32(&recoveredCount, 1)
					}
				}
				tx.Commit()
				buffer = make([]updateResult, 0, batchSize)
			}

			for {
				select {
				case res, ok := <-resultChan:
					if !ok {
						flush() // Flush remaining items
						return
					}
					buffer = append(buffer, res)
					if len(buffer) >= batchSize {
						flush()
					}
				case <-ticker.C:
					flush()
				}
			}
		}()

		// Phase 1: scan and collect missing files
		for _, img := range images {
			expectedFullPath := filepath.Join(UploadsDir, img.Filename)

			if _, err := os.Stat(expectedFullPath); os.IsNotExist(err) {
				missingCount++

				// Optional: regenerate thumbnail if main file exists but thumb doesn't
				thumbPath := filepath.Join(UploadsDir, "thumbnails", filepath.Base(img.Filename))
				if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
					if _, err := os.Stat(expectedFullPath); err == nil {
						go GenerateThumbnail(expectedFullPath)
					}
				}

				if img.DownloadURL == "" {
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
					// Use Find() instead of First() to avoid "record not found" errors
					if database.DB.Select("name").Find(&src, *sourceID).RowsAffected > 0 {
						sourceName = src.Name
					}
					// If source not found (deleted), sourceName remains "uncategorized"
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

		if missingCount > 0 {
			fmt.Printf("Found %d missing images\n", missingCount)
		}

		// Phase 2: concurrent recovery – THE IMPORTANT PART
		if len(tasks) > 0 {

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
					result, err := DownloadImage(t.DownloadURL, t.SourceName)
					if err != nil {
						return
					}

					// We now trust DownloadImage to handle hashing and storage consistently.
					// We just need to calculate the relative path for the DB.
					// result.Path is like "uploads\Source\hash.jpg"
					// We want "Source\hash.jpg"
					relPath, err := filepath.Rel(UploadsDir, result.Path)
					if err != nil {
						relPath = filepath.Join(t.SourceName, filepath.Base(result.Path)) // Fallback
					}

					// Send result to batch processor instead of updating directly
					resultChan <- updateResult{
						ID:             t.ID,
						RelPath:        relPath,
						DominantColors: result.DominantColors,
					}

					// Generate thumbnail
					GenerateThumbnail(result.Path)

					duration := time.Since(start)
					if duration > 5*time.Second {
						time.Sleep(1 * time.Second) // polite pause
					}
				}(task)
			}

			wg.Wait()
			close(resultChan) // Signal batch processor to finish
			wgBatch.Wait()    // Wait for batch processor to complete writes
		}

		recovered := atomic.LoadInt32(&recoveredCount)
		stillMissing := missingCount - int(recovered)
		if missingCount > 0 {
			fmt.Printf("Image verification complete — Downloaded: %d | Still missing: %d\n", recovered, stillMissing)
		}
	}()

	return nil
}

// VerifyPersonImages checks all people with a StashID and re-downloads missing photos
func VerifyPersonImages() error {
	logger.Info("Verifying person images...")

	var people []models.Person
	// Find all people that are linked to StashDB
	if err := database.DB.Where("stash_id != ? AND stash_id != ''", "").Find(&people).Error; err != nil {
		return fmt.Errorf("failed to query people: %w", err)
	}

	go func() {
		var recoveredCount int32 = 0

		for _, person := range people {
			if person.Photos == "" {
				continue
			}

			var photoPaths []string
			if err := json.Unmarshal([]byte(person.Photos), &photoPaths); err != nil {
				continue
			}

			needsRecovery := false
			for _, webPath := range photoPaths {
				// webPath is like "/person-images/1/abc.jpg"
				// We need to convert it to "uploads/person_images/1/abc.jpg"

				// Remove the leading "/" if present
				relativePath := strings.TrimPrefix(webPath, "/")
				if strings.HasPrefix(relativePath, "person-images/") {
					// It's in our expected format.
					// We need to map "person-images" to "uploads/person_images" technically?
					// Wait, DownloadPersonImage returns `/person-images/...`
					// And `r.Static("/person-images", "./uploads/person_images")` in main.go
					// So `/person-images/` on web maps to `./uploads/person_images/` on disk.

					// Replace "person-images" with "uploads/person_images" for filesystem check
					// BUT filepath.Join handles separators.
					// Let's strip "person-images/" and join with UploadsDir + "person_images"

					parts := strings.SplitN(relativePath, "/", 2)
					if len(parts) == 2 {
						// parts[1] is "1/abc.jpg"
						// On windows this needs to become "uploads\person_images\1\abc.jpg"
						localPath := filepath.Join(UploadsDir, "person_images", filepath.FromSlash(parts[1]))
						if _, err := os.Stat(localPath); os.IsNotExist(err) {
							fmt.Printf("Missing person image: %s (Person ID: %d)\n", localPath, person.ID)
							needsRecovery = true
							break
						}
					}
				}
			}

			if needsRecovery {
				fmt.Printf("Recovering images for person %s (ID: %d, StashID: %s)...\n", person.Name, person.ID, person.StashID)

				stashService := NewStashDBService()
				performer, err := stashService.GetPerformer(person.StashID)
				if err != nil {
					fmt.Printf("Failed to fetch performer %s from StashDB: %v\n", person.StashID, err)
					continue
				}

				var newPhotoURLs []string
				personIDUint := person.ID

				for _, img := range performer.Images {
					localPath, err := DownloadPersonImage(img.URL, personIDUint)
					if err != nil {
						fmt.Printf("Failed to re-download image %s: %v\n", img.URL, err)
						continue
					}
					newPhotoURLs = append(newPhotoURLs, localPath)
				}

				if len(newPhotoURLs) > 0 {
					photosJSON, _ := json.Marshal(newPhotoURLs)

					// Update DB
					if err := database.DB.Model(&models.Person{ID: person.ID}).Update("photos", string(photosJSON)).Error; err != nil {
						fmt.Printf("Failed to update person photos for ID %d: %v\n", person.ID, err)
					} else {
						atomic.AddInt32(&recoveredCount, 1)
						fmt.Printf("Successfully recovered images for person ID %d\n", person.ID)
					}
				}
			}
		}

		if recoveredCount > 0 {
			logger.Info(fmt.Sprintf("Completed verification of person images. Recovered: %d people.", recoveredCount))
		} else {
			logger.Info("Person image verification complete. No missing images found.")
		}
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
