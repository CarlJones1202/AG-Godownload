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
						logger.Infof("Local video file no longer exists, skipping: %s (ID: %d)", localPath, video.ID)
						atomic.AddInt32(&skippedCount, 1)
						continue
					}
					// Local file exists, we could re-import it, but for now just log
					logger.Infof("Local video file still exists but not in expected location: %s (ID: %d)", localPath, video.ID)
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
						logger.Warnf("[Video ID %d] Re-download failed: %v", t.ID, err)
						atomic.AddInt32(&skippedCount, 1)
						return
					}

					// Calculate relative path for DB
					relPath, err := filepath.Rel(UploadsDir, result.Path)
					if err != nil {
						logger.Warnf("[Video ID %d] Failed to get relative path: %v", t.ID, err)
						relPath = filepath.Join(t.SourceName, filepath.Base(result.Path)) // Fallback
					}

					// Update database
					if err := database.DB.
						Model(&models.Image{ID: t.ID}).
						Updates(map[string]interface{}{
							"filename": relPath,
							"duration": result.Duration,
							"width":    result.Width,
							"height":   result.Height,
							"size_mb":  result.SizeMB,
						}).Error; err != nil {

						logger.Warnf("[Video ID %d] Failed to update DB: %v", t.ID, err)
						atomic.AddInt32(&skippedCount, 1)
					} else {
						logger.Infof("[Video ID %d] Recovered → %s (%dx%d, %.1fs, %.1fMB)",
							t.ID, relPath, result.Width, result.Height, result.Duration, result.SizeMB)
						atomic.AddInt32(&recoveredCount, 1)
					}

					// Generate thumbnail (DownloadVideo already does this, but just in case)
					if _, err := GenerateVideoThumbnail(result.Path); err != nil {
						logger.Warnf("[Video ID %d] Thumbnail generation failed: %v", t.ID, err)
					}

					// Generate trickplay data (DownloadVideo already does this, but just in case)
					if err := GenerateTrickplayData(result.Path); err != nil {
						logger.Warnf("[Video ID %d] Trickplay generation failed: %v", t.ID, err)
					}

					duration := time.Since(start)
					if duration > 10*time.Second {
						time.Sleep(2 * time.Second) // polite pause for large downloads
					}
				}(task)
			}

			wg.Wait()
		}

		logger.Infof("Video verification complete — Missing: %d | Recovered: %d | Skipped: %d",
			missingCount, atomic.LoadInt32(&recoveredCount), atomic.LoadInt32(&skippedCount))
	}()

	return nil
}
