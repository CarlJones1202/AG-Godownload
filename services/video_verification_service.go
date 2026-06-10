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

	// Count first so the UI can show progress.
	var total int64
	if err := database.DB.Model(&models.Image{}).Where("type = ?", "video").Count(&total).Error; err != nil {
		return fmt.Errorf("failed to count videos: %w", err)
	}

	go func() {
		SetVideoVerificationRunning(true, int(total))
		defer SetVideoVerificationRunning(false, 0)

		missingCount := 0
		var recoveredCount int32
		var skippedCount int32

		type recoveryTask struct {
			ID            uint
			CurrentDBPath string
			DownloadURL   string
			OriginalURL   string
			SourceName    string
			Title         string
		}

		var tasks []recoveryTask
		var mu sync.Mutex

		type videoUpdateResult struct {
			ID       uint
			RelPath  string
			Duration float64
			Width    int
			Height   int
			SizeMB   float64
		}

		resultChan := make(chan videoUpdateResult, 50)
		var wgBatch sync.WaitGroup

		// Batch writer: collects results and writes periodically or when full.
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
					if err := tx.Unscoped().Model(&models.Image{}).
						Where("id = ?", res.ID).
						Updates(map[string]interface{}{
							"filename": res.RelPath,
							"duration": res.Duration,
							"width":    res.Width,
							"height":   res.Height,
							"size_mb":  res.SizeMB,
						}).Error; err != nil {
						logger.Errorf("Failed to update video %d: %v", res.ID, err)
					} else {
						atomic.AddInt32(&recoveredCount, 1)
						IncVideoVerificationRecovered()
					}
				}
				tx.Commit()
				buffer = buffer[:0]
			}

			for {
				select {
				case res, ok := <-resultChan:
					if !ok {
						flush()
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

		// Phase 1: scan and collect missing files (chunked)
		const chunkSize = 100
		var lastID uint
		for {
			var videos []struct {
				ID          uint
				Filename    string
				DownloadURL string
				OriginalURL string
				Title       string
			}

			q := database.DB.Model(&models.Image{}).
				Select("id, filename, download_url, original_url, title").
				Where("type = ? AND id > ?", "video", lastID).
				Order("id ASC").
				Limit(chunkSize)

			if err := q.Find(&videos).Error; err != nil {
				logger.Errorf("failed to query video batch: %v", err)
				break
			}
			if len(videos) == 0 {
				break
			}

			for _, v := range videos {
				IncVideoVerificationProcessed()
				expectedFullPath := filepath.Join(UploadsDir, v.Filename)

				// Regenerate thumbnail if missing
				thumbFilename := strings.TrimSuffix(filepath.Base(v.Filename), filepath.Ext(v.Filename)) + ".jpg"
				thumbPath := filepath.Join(UploadsDir, filepath.Dir(v.Filename), "thumbnails", thumbFilename)
				if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
					if _, err2 := os.Stat(expectedFullPath); err2 == nil {
						go GenerateVideoThumbnail(expectedFullPath)
					}
				}

				if _, err := os.Stat(expectedFullPath); os.IsNotExist(err) {
					missingCount++
					IncVideoVerificationMissing()
					fmt.Printf("Missing video: %s (ID: %d)\n", v.Filename, v.ID)

					// Local file handling
					if strings.HasPrefix(v.OriginalURL, "file://") {
						localPath := strings.TrimPrefix(v.OriginalURL, "file://")
						if _, err := os.Stat(localPath); os.IsNotExist(err) {
							logger.Infof("[Source: %s] Local video file no longer exists at original path: %s (ID: %d)", v.OriginalURL, localPath, v.ID)
							logger.Infof("[Source: %s] Expected location in uploads: %s", v.OriginalURL, expectedFullPath)
							atomic.AddInt32(&skippedCount, 1)
							continue
						}
						logger.Infof("[Source: %s] Local video file still exists at original path but not in expected location", v.OriginalURL)
						atomic.AddInt32(&skippedCount, 1)
						continue
					}

					if v.DownloadURL == "" {
						logger.Infof("No DownloadURL for video ID %d → cannot recover", v.ID)
						atomic.AddInt32(&skippedCount, 1)
						continue
					}

					// Resolve source name
					sourceName := "uncategorized"
					var sourceID *uint
					err := database.DB.Table("galleries").
						Select("galleries.source_id").
						Joins("JOIN image_galleries ON image_galleries.gallery_id = galleries.id").
						Where("image_galleries.image_id = ?", v.ID).
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
						ID:            v.ID,
						CurrentDBPath: v.Filename,
						DownloadURL:   v.DownloadURL,
						OriginalURL:   v.OriginalURL,
						SourceName:    sourceName,
						Title:         v.Title,
					})
					mu.Unlock()
				}
			}

			// keep scanning from last ID in this batch
			lastID = videos[len(videos)-1].ID
		}

		// Phase 2: concurrent recovery
		if len(tasks) > 0 {
			fmt.Printf("Recovering %d missing videos (max 10 concurrent)...\n", len(tasks))
			const maxConcurrent = 10
			sem := make(chan struct{}, maxConcurrent)
			var wg sync.WaitGroup

			for _, t := range tasks {
				wg.Add(1)
				go func(task recoveryTask) {
					defer wg.Done()
					UpdateVideoActiveCount(1)
					AddActiveVideoDownload(task.ID, task.Title, task.DownloadURL, task.SourceName)
					sem <- struct{}{}
					defer func() {
						<-sem
						UpdateVideoActiveCount(-1)
						RemoveActiveVideoDownload(task.ID)
					}()

					start := time.Now()
					pageURL := task.OriginalURL
					if pageURL == "" {
						pageURL = task.DownloadURL
					}

					result, err := DownloadVideo(task.DownloadURL, task.SourceName, pageURL, task.Title)
					if err != nil {
						// try to refresh URLs for some providers
						refreshed := false
						if task.OriginalURL != "" {
							var newVideoURL, newTitle string
							var ripErr error
							if strings.Contains(task.OriginalURL, "tnaflix.com") {
								newVideoURL, newTitle, ripErr = RipTnaFlix(task.OriginalURL)
							} else if strings.Contains(task.OriginalURL, "pornhub.com") {
								newVideoURL, newTitle, ripErr = RipPornhub(task.OriginalURL)
							}
							if ripErr == nil && newVideoURL != "" {
								database.DB.Model(&models.Image{ID: task.ID}).Update("download_url", newVideoURL)
								result, err = DownloadVideo(newVideoURL, task.SourceName, pageURL, newTitle)
								if err == nil {
									refreshed = true
								}
							}
						}
						if !refreshed {
							logger.Warnf("[Video ID %d] [Source: %s] Re-download failed: %v", task.ID, task.SourceName, err)
							atomic.AddInt32(&skippedCount, 1)
							return
						}
					}

					relPath, err := filepath.Rel(UploadsDir, result.Path)
					if err != nil {
						logger.Warnf("[Video ID %d] [Source: %s] Failed to get relative path: %v", task.ID, task.SourceName, err)
						relPath = filepath.Join(task.SourceName, filepath.Base(result.Path))
					}

					// send to batch writer
					resultChan <- videoUpdateResult{
						ID:       task.ID,
						RelPath:  relPath,
						Duration: result.Duration,
						Width:    result.Width,
						Height:   result.Height,
						SizeMB:   result.SizeMB,
					}

					// extra processing
					if _, err := GenerateVideoThumbnail(result.Path); err != nil {
						logger.Warnf("[Video ID %d] Thumbnail generation failed: %v", task.ID, err)
					}
					if err := GenerateTrickplayData(result.Path); err != nil {
						logger.Warnf("[Video ID %d] Trickplay generation failed: %v", task.ID, err)
					}

					dur := time.Since(start)
					if dur > 10*time.Second {
						time.Sleep(2 * time.Second)
					}
				}(t)
			}

			wg.Wait()
			close(resultChan)
			wgBatch.Wait()
			database.Checkpoint()
		}

		logger.Infof("Video verification complete — Missing: %d | Recovered: %d | Skipped: %d",
			missingCount, atomic.LoadInt32(&recoveredCount), atomic.LoadInt32(&skippedCount))
	}()

	return nil
}
