package services

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// VideoRecoveryTask represents a missing video task to be recovered
type VideoRecoveryTask struct {
	ID            uint
	CurrentDBPath string
	DownloadURL   string
	OriginalURL   string
	SourceName    string
	Title         string
	SizeMB        float64
	ThumbnailTime *float64
}

// SortVideoRecoveryTasks sorts tasks by SizeMB ascending (smallest first).
// If sizes are equal or unknown, it falls back to ID ascending.
func SortVideoRecoveryTasks(tasks []VideoRecoveryTask) {
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].SizeMB != tasks[j].SizeMB {
			return tasks[i].SizeMB < tasks[j].SizeMB
		}
		return tasks[i].ID < tasks[j].ID
	})
}

// videoScanRow is one missing-video row plus its source links. The LEFT JOINs
// can duplicate a row when a video belongs to several galleries; callers must
// dedupe by ID.
type videoScanRow struct {
	ID              uint
	Filename        string
	DownloadURL     string
	OriginalURL     string
	Title           string
	SizeMB          float64
	ThumbnailTime   *float64
	DirectSourceID  *uint
	GallerySourceID *uint
}

// collectMissingVideoTasks selects video rows whose files are not on disk,
// using the file_exists column that SyncFileExists computes immediately before
// recovery runs at startup (callers must run it first — no per-row stat or
// UPDATE is performed here). Source names are resolved from the row's own
// direct source_id first and the linked gallery's source as fallback, using
// one chunked query per batch plus a single source-name lookup instead of two
// queries per row. Returns the recovery tasks, the number of missing rows, the
// number skipped as unrecoverable, and the first query error encountered (with
// everything collected up to that point).
func collectMissingVideoTasks() ([]VideoRecoveryTask, int, int, error) {
	const chunkSize = 500
	var lastID uint
	missing := 0
	skipped := 0
	var firstErr error

	var candidates []videoScanRow
	sourceIDFor := make(map[uint]uint) // image ID -> source ID (direct link wins)

	for {
		var rows []videoScanRow
		if err := database.DB.
			Model(&models.Image{}).
			Select("images.id, images.filename, images.download_url, images.original_url, images.title, images.size_mb, images.thumbnail_time_seconds, images.source_id AS direct_source_id, galleries.source_id AS gallery_source_id").
			Joins("LEFT JOIN image_galleries ON image_galleries.image_id = images.id").
			Joins("LEFT JOIN galleries ON galleries.id = image_galleries.gallery_id").
			Where("images.type = ? AND images.file_exists = ? AND images.id > ?", "video", false, lastID).
			Order("images.id ASC").
			Limit(chunkSize).
			Find(&rows).Error; err != nil {
			if firstErr == nil {
				firstErr = err
			}
			logger.Errorf("failed to query missing video batch: %v", err)
			break
		}
		if len(rows) == 0 {
			break
		}

		var prevID uint
		for _, v := range rows {
			// Dedupe gallery-join duplicates within the page (keyset
			// pagination guarantees duplicates never span pages).
			if v.ID == prevID {
				continue
			}
			prevID = v.ID

			missing++
			IncVideoVerificationProcessed()
			IncVideoVerificationMissing()

			// Local file handling
			if strings.HasPrefix(v.OriginalURL, "file://") {
				localPath := strings.TrimPrefix(v.OriginalURL, "file://")
				if _, err := os.Stat(localPath); os.IsNotExist(err) {
					logger.Infof("[Source: %s] Local video file no longer exists at original path: %s (ID: %d)", v.OriginalURL, localPath, v.ID)
					logger.Infof("[Source: %s] Expected location in uploads: %s", v.OriginalURL, filepath.Join(UploadsDir, v.Filename))
				} else {
					logger.Infof("[Source: %s] Local video file still exists at original path but not in expected location", v.OriginalURL)
				}
				skipped++
				continue
			}

			if v.DownloadURL == "" {
				logger.Infof("No DownloadURL for video ID %d → cannot recover", v.ID)
				skipped++
				continue
			}

			if v.DirectSourceID != nil {
				sourceIDFor[v.ID] = *v.DirectSourceID
			} else if v.GallerySourceID != nil {
				if _, seen := sourceIDFor[v.ID]; !seen {
					sourceIDFor[v.ID] = *v.GallerySourceID
				}
			}

			candidates = append(candidates, v)
		}

		lastID = rows[len(rows)-1].ID
	}

	// One source-name lookup for every source referenced by any candidate.
	sourceNames := make(map[uint]string, len(sourceIDFor))
	if len(sourceIDFor) > 0 {
		seen := make(map[uint]bool, len(sourceIDFor))
		sourceIDs := make([]uint, 0, len(sourceIDFor))
		for _, sid := range sourceIDFor {
			if !seen[sid] {
				seen[sid] = true
				sourceIDs = append(sourceIDs, sid)
			}
		}
		var sources []struct {
			ID   uint
			Name string
		}
		if err := database.DB.Model(&models.Source{}).
			Select("id, name").
			Where("id IN ?", sourceIDs).
			Find(&sources).Error; err != nil {
			if firstErr == nil {
				firstErr = err
			}
			logger.Errorf("failed to resolve video source names: %v", err)
		} else {
			for _, s := range sources {
				sourceNames[s.ID] = s.Name
			}
		}
	}

	tasks := make([]VideoRecoveryTask, 0, len(candidates))
	for _, v := range candidates {
		sourceName := "uncategorized"
		if sid, ok := sourceIDFor[v.ID]; ok {
			if name, ok := sourceNames[sid]; ok && name != "" {
				sourceName = name
			}
		}
		tasks = append(tasks, VideoRecoveryTask{
			ID:            v.ID,
			CurrentDBPath: v.Filename,
			DownloadURL:   v.DownloadURL,
			OriginalURL:   v.OriginalURL,
			SourceName:    sourceName,
			Title:         v.Title,
			SizeMB:        v.SizeMB,
			ThumbnailTime: v.ThumbnailTime,
		})
	}

	return tasks, missing, skipped, firstErr
}

// regeneratePresentVideoThumbnails scans videos whose files are present
// (file_exists = true) and submits thumbnail regeneration for any whose
// thumbnail is missing. Stat work is bounded by statConcurrency; the
// CPU-heavy generation runs on the post pool. This pass replaces the per-row
// thumbnail stat that used to run inside the main scan loop. It also advances
// the processed counter for present rows so the progress total is reached.
func regeneratePresentVideoThumbnails(postChan chan func()) {
	const chunkSize = 500
	var lastID uint
	var wg sync.WaitGroup
	sem := make(chan struct{}, statConcurrency)

	for {
		var rows []struct {
			ID            uint
			Filename      string
			ThumbnailTime *float64
		}
		if err := database.DB.Model(&models.Image{}).
			Select("id, filename, thumbnail_time_seconds").
			Where("type = ? AND file_exists = ? AND id > ?", "video", true, lastID).
			Order("id ASC").
			Limit(chunkSize).
			Find(&rows).Error; err != nil {
			logger.Errorf("failed to query present video batch: %v", err)
			return
		}
		if len(rows) == 0 {
			break
		}

		for _, r := range rows {
			IncVideoVerificationProcessed()
			wg.Add(1)
			go func(id uint, filename string, thumbnailTime *float64) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				expectedFullPath := filepath.Join(UploadsDir, filename)
				thumbFilename := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)) + ".jpg"
				thumbPath := filepath.Join(UploadsDir, filepath.Dir(filename), "thumbnails", thumbFilename)
				if _, err := os.Stat(thumbPath); os.IsNotExist(err) {
					if _, err2 := os.Stat(expectedFullPath); err2 == nil {
						postChan <- func() {
							if _, err := GenerateVideoThumbnailAt(expectedFullPath, thumbnailTime); err != nil {
								logger.Warnf("[Video ID %d] Thumbnail generation failed: %v", id, err)
							}
						}
					}
				}
			}(r.ID, r.Filename, r.ThumbnailTime)
		}
		wg.Wait()
		lastID = rows[len(rows)-1].ID
	}
}

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

		var recoveredCount int32
		var skippedCount int32

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
							"filename":    res.RelPath,
							"duration":    res.Duration,
							"width":       res.Width,
							"height":      res.Height,
							"size_mb":     res.SizeMB,
							"file_exists": true,
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

		// Post-processing pool: video thumbnail + trickplay generation are
		// CPU-bound (trickplay can block a download slot for minutes) and
		// must not run inline with downloads.
		postChan := make(chan func(), 128)
		var wgPost sync.WaitGroup
		for w := 0; w < recoveryPostprocessWorkers(); w++ {
			wgPost.Add(1)
			go func() {
				defer wgPost.Done()
				for job := range postChan {
					job()
				}
			}()
		}

		// Thumbnail regeneration for videos already on disk runs alongside
		// collection and downloads instead of blocking recovery start.
		var wgPresent sync.WaitGroup
		wgPresent.Add(1)
		go func() {
			defer wgPresent.Done()
			regeneratePresentVideoThumbnails(postChan)
		}()

		// Phase 1: collect missing files from the file_exists column, which
		// SyncFileExists computed immediately before this runs at startup.
		tasks, missingCount, skipped, collectErr := collectMissingVideoTasks()
		if collectErr != nil {
			logger.Errorf("Missing-video scan finished with errors: %v", collectErr)
		}
		atomic.AddInt32(&skippedCount, int32(skipped))
		if missingCount > 0 {
			logger.Infof("Found %d missing videos (%d not recoverable)", missingCount, skipped)
		}

		// Phase 2: prioritize (smallest first) and concurrent recovery
		if len(tasks) > 0 {
			SortVideoRecoveryTasks(tasks)
			maxConcurrent := recoveryConcurrency()
			logger.Infof("Recovering %d missing videos (smallest first, max %d concurrent)...", len(tasks), maxConcurrent)
			numWorkers := maxConcurrent
			if len(tasks) < numWorkers {
				numWorkers = len(tasks)
			}

			taskChan := make(chan VideoRecoveryTask, len(tasks))
			for _, t := range tasks {
				taskChan <- t
			}
			close(taskChan)

			var wg sync.WaitGroup
			for w := 0; w < numWorkers; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for task := range taskChan {
						func(t VideoRecoveryTask) {
							UpdateVideoActiveCount(1)
							AddActiveVideoDownload(t.ID, t.Title, t.DownloadURL, t.SourceName)
							defer func() {
								UpdateVideoActiveCount(-1)
								RemoveActiveVideoDownload(t.ID)
							}()

							start := time.Now()
							pageURL := t.OriginalURL
							if pageURL == "" {
								pageURL = t.DownloadURL
							}

							result, err := DownloadVideoWithoutPostProcessing(t.DownloadURL, t.SourceName, pageURL, t.Title)
							if err != nil {
								// try to refresh URLs for some providers
								refreshed := false
								if t.OriginalURL != "" {
									var newVideoURL, newTitle string
									var ripErr error
									if strings.Contains(t.OriginalURL, "tnaflix.com") {
										newVideoURL, newTitle, ripErr = RipTnaFlix(t.OriginalURL)
									} else if strings.Contains(t.OriginalURL, "pornhub.com") {
										newVideoURL, newTitle, ripErr = RipPornhub(t.OriginalURL)
									} else if isYouTubeURL(t.OriginalURL) {
										// YouTube stream URLs go stale; re-rip the page and import the fresh file
										result, err = DownloadVideoWithoutPostProcessing(t.OriginalURL, t.SourceName, t.OriginalURL, t.Title)
										if err == nil {
											refreshed = true
										}
									}
									if ripErr == nil && newVideoURL != "" {
										database.DB.Model(&models.Image{ID: t.ID}).Update("download_url", newVideoURL)
										result, err = DownloadVideoWithoutPostProcessing(newVideoURL, t.SourceName, pageURL, newTitle)
										if err == nil {
											refreshed = true
										}
									}
								}
								if !refreshed {
									logger.Warnf("[Video ID %d] [Source: %s] Re-download failed: %v", t.ID, t.SourceName, err)
									atomic.AddInt32(&skippedCount, 1)
									return
								}
							}

							relPath, err := filepath.Rel(UploadsDir, result.Path)
							if err != nil {
								logger.Warnf("[Video ID %d] [Source: %s] Failed to get relative path: %v", t.ID, t.SourceName, err)
								relPath = filepath.Join(t.SourceName, filepath.Base(result.Path))
							}

							// DB update goes to the batch writer immediately;
							// heavy post-processing is handed to the pool.
							resultChan <- videoUpdateResult{
								ID:       t.ID,
								RelPath:  relPath,
								Duration: result.Duration,
								Width:    result.Width,
								Height:   result.Height,
								SizeMB:   result.SizeMB,
							}

							postPath := result.Path
							thumbnailTime := t.ThumbnailTime
							postChan <- func() {
								if _, err := GenerateVideoThumbnailAt(postPath, thumbnailTime); err != nil {
									logger.Warnf("[Video ID %d] Thumbnail generation failed: %v", t.ID, err)
								}
								if err := GenerateTrickplayData(postPath); err != nil {
									logger.Warnf("[Video ID %d] Trickplay generation failed: %v", t.ID, err)
								}
							}

							dur := time.Since(start)
							if dur > 10*time.Second {
								time.Sleep(2 * time.Second)
							}
						}(task)
					}
				}()
			}

			wg.Wait()
		}

		// All submitters are done: drain post-processing, then the batch writer.
		wgPresent.Wait()
		close(postChan)
		wgPost.Wait()
		close(resultChan)
		wgBatch.Wait()
		database.Checkpoint()

		logger.Infof("Video verification complete — Missing: %d | Recovered: %d | Skipped: %d",
			missingCount, atomic.LoadInt32(&recoveredCount), atomic.LoadInt32(&skippedCount))
	}()

	return nil
}
