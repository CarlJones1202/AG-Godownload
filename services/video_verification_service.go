package services

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// VerifyDownloadedVideos checks all videos in the database and re-downloads any missing files
func VerifyDownloadedVideos() error {
	logger.Info("Verifying downloaded videos...")

	var videos []struct {
		ID          uint
		Filename    string // current value in DB (may point to missing/broken file)
		DownloadURL string
		OriginalURL string
		Title       string
	}

	if err := database.DB.
		Model(&models.Image{}).
		Select("id, filename, download_url, original_url, title").
		Where("type = ?", "video").
		Find(&videos).Error; err != nil {
		return fmt.Errorf("failed to query videos: %w", err)
	}

	go func() {
		missingCount := 0
		var recoveredCount int32 = 0
		var skippedCount int32 = 0

		type recoveryTask struct {
			ID            uint
			CurrentDBPath string // what DB currently thinks the path is
			DownloadURL   string
			OriginalURL   string
			SourceName    string
			Title         string
		}

		var tasks []recoveryTask

		var mu sync.Mutex

		// Channel for batched updates
		type videoUpdateResult struct {
			ID       uint
			RelPath  string
			Duration float64
			Width    int
			Height   int
			SizeMB   float64
		}
		resultChan := make(chan videoUpdateResult, 50) // Smaller buffer as videos process slower
		var wgBatch sync.WaitGroup

		// Start batch processor
		wgBatch.Add(1)
		go func() {
			defer wgBatch.Done()
			var buffer []videoUpdateResult
			const batchSize = 20
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
							"filename": res.RelPath,
							"duration": res.Duration,
							"width":    res.Width,
							"height":   res.Height,
							"size_mb":  res.SizeMB,
						}).Error; err != nil {
						logger.Errorf("Failed to update video %d in batch: %v", res.ID, err)
					} else {
						atomic.AddInt32(&recoveredCount, 1)
					}
				}
				tx.Commit()
				buffer = make([]videoUpdateResult, 0, batchSize)
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
		for _, video := range videos {
			expectedFullPath := filepath.Join(UploadsDir, video.Filename)

			if _, err := os.Stat(expectedFullPath); os.IsNotExist(err) {
				missingCount++
				fmt.Printf("Missing video: %s (ID: %d)\n", video.Filename, video.ID)

				// Check if this is a local video file
				if strings.HasPrefix(video.OriginalURL, "file://") {
					// Local video - check if original path exists
					localPath := strings.TrimPrefix(video.OriginalURL, "file://")
					if _, err := os.Stat(localPath); os.IsNotExist(err) {
						// Local file no longer exists, skip gracefully
						logger.Infof("[Source: %s] Local video file no longer exists at original path: %s (ID: %d)", video.OriginalURL, localPath, video.ID)
						logger.Infof("[Source: %s] Expected location in uploads: %s", video.OriginalURL, expectedFullPath)
						atomic.AddInt32(&skippedCount, 1)
						continue
					}
					// Local file exists, we could re-import it, but for now just log
					logger.Infof("[Source: %s] Local video file still exists at original path but not in expected location", video.OriginalURL)
					logger.Infof("[Source: %s] Original path: %s", video.OriginalURL, localPath)
					logger.Infof("[Source: %s] Expected location: %s (ID: %d)", video.OriginalURL, expectedFullPath, video.ID)
					atomic.AddInt32(&skippedCount, 1)
					continue
				}

				// Check for thumbnail and regenerate if main file exists but thumb doesn't
				thumbPath := filepath.Join(UploadsDir, filepath.Dir(video.Filename), "thumbnails", filepath.Base(video.Filename)+".jpg")
				if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
					if _, err := os.Stat(expectedFullPath); err == nil {
						go GenerateVideoThumbnail(expectedFullPath)
					}
				}

				if video.DownloadURL == "" {
					logger.Infof("No DownloadURL for video ID %d → cannot recover", video.ID)
					atomic.AddInt32(&skippedCount, 1)
					continue
				}

				// Resolve source name (same logic as image verification)
				sourceName := "uncategorized"
				var sourceID *uint
				err := database.DB.
					Table("galleries").
					Select("galleries.source_id").
					Joins("JOIN image_galleries ON image_galleries.gallery_id = galleries.id").
					Where("image_galleries.image_id = ?", video.ID).
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
					ID:            video.ID,
					CurrentDBPath: video.Filename,
					DownloadURL:   video.DownloadURL,
					OriginalURL:   video.OriginalURL,
					SourceName:    sourceName,
					Title:         video.Title,
				})
				mu.Unlock()
			}
		}

		// Phase 2: concurrent recovery with limited concurrency for videos
		if len(tasks) > 0 {
			fmt.Printf("Recovering %d missing videos (max 3 concurrent)...\n", len(tasks))

			const maxConcurrent = 3 // Lower than images due to large file sizes
			sem := make(chan struct{}, maxConcurrent)
			var wg sync.WaitGroup

			for _, task := range tasks {
				wg.Add(1)
				go func(t recoveryTask) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()

					start := time.Now()

					// Determine the page URL for the download
					pageURL := t.OriginalURL
					if pageURL == "" {
						pageURL = t.DownloadURL
					}

					// Download the video
					result, err := DownloadVideo(t.DownloadURL, t.SourceName, pageURL, t.Title)
					if err != nil {
						// Attempt to refresh URL if download failed
						refreshed := false
						if t.OriginalURL != "" {
							var newVideoURL, newTitle string
							var ripErr error

							if strings.Contains(t.OriginalURL, "tnaflix.com") {
								logger.Infof("[Video ID %d] Attempting to refresh TnaFlix URL from %s", t.ID, t.OriginalURL)
								newVideoURL, newTitle, ripErr = RipTnaFlix(t.OriginalURL)
							} else if strings.Contains(t.OriginalURL, "pornhub.com") {
								logger.Infof("[Video ID %d] Attempting to refresh Pornhub URL from %s", t.ID, t.OriginalURL)
								newVideoURL, newTitle, ripErr = RipPornhub(t.OriginalURL)
							}

							if ripErr == nil && newVideoURL != "" {
								logger.Infof("[Video ID %d] Successfully refreshed URL. Retrying download...", t.ID)

								// Update DB with new URL
								database.DB.Model(&models.Image{ID: t.ID}).Update("download_url", newVideoURL)

								// Retry download
								result, err = DownloadVideo(newVideoURL, t.SourceName, pageURL, newTitle)
								if err == nil {
									refreshed = true
								}
							} else if ripErr != nil {
								logger.Warnf("[Video ID %d] Failed to refresh URL: %v", t.ID, ripErr)
							}
						}

						if !refreshed {
							logger.Warnf("[Video ID %d] [Source: %s] [Page: %s] [DL: %s] Re-download failed: %v", t.ID, t.SourceName, t.OriginalURL, t.DownloadURL, err)
							atomic.AddInt32(&skippedCount, 1)
							return
						}
					}

					// Calculate relative path for DB
					relPath, err := filepath.Rel(UploadsDir, result.Path)
					if err != nil {
						logger.Warnf("[Video ID %d] [Source: %s] Failed to get relative path: %v", t.ID, t.SourceName, err)
						relPath = filepath.Join(t.SourceName, filepath.Base(result.Path)) // Fallback
					}

					// Send result to batch processor
					resultChan <- videoUpdateResult{
						ID:       t.ID,
						RelPath:  relPath,
						Duration: result.Duration,
						Width:    result.Width,
						Height:   result.Height,
						SizeMB:   result.SizeMB,
					}

					logger.Infof("[Video ID %d] [Source: %s] [Page: %s] Recovered → %s (%dx%d, %.1fs, %.1fMB)",
						t.ID, t.SourceName, t.OriginalURL, relPath, result.Width, result.Height, result.Duration, result.SizeMB)

					// Generate thumbnail (DownloadVideo already does this, but just in case)
					if _, err := GenerateVideoThumbnail(result.Path); err != nil {
						logger.Warnf("[Video ID %d] [Source: %s] Thumbnail generation failed: %v", t.ID, t.SourceName, err)
					}

					// Generate trickplay data (DownloadVideo already does this, but just in case)
					if err := GenerateTrickplayData(result.Path); err != nil {
						logger.Warnf("[Video ID %d] [Source: %s] Trickplay generation failed: %v", t.ID, t.SourceName, err)
					}

					duration := time.Since(start)
					if duration > 10*time.Second {
						time.Sleep(2 * time.Second) // polite pause for large downloads
					}
				}(task)
			}

			wg.Wait()
			close(resultChan) // Signal batch processor to finish
			wgBatch.Wait()    // Wait for batch processor to complete writes
		}

		logger.Infof("Video verification complete — Missing: %d | Recovered: %d | Skipped: %d",
			missingCount, atomic.LoadInt32(&recoveredCount), atomic.LoadInt32(&skippedCount))
	}()

	return nil
}
