package services

import (
	"errors"
	"fmt"
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"
)

const maxEmbedRetries = 4

// ErrImageFileMissing reports that an image's file is not on disk yet. It is a
// transient condition (fresh restart before re-download), not a permanent
// failure, so the worker parks the row as "deferred" instead of burning retries.
var ErrImageFileMissing = errors.New("image file not found on disk")

// EmbedWorkQueue feeds embedding workers. The embed_queues table is the source
// of truth for pending work across restarts; dropped channel sends are picked
// up again by the startup and periodic sweeps.
var EmbedWorkQueue = make(chan uint, 1024)

// EnqueueEmbed schedules an image for embedding + tagging. It doubles as the
// "this file is available now" signal: terminal (failed) and deferred rows are
// reset to pending so a re-downloaded image gets embedded, while work that is
// already scheduled or in flight is left alone. Idempotent.
func EnqueueEmbed(imageID uint) {
	if err := database.DB.Exec(
		"INSERT OR IGNORE INTO embed_queues (image_id, status, attempts) VALUES (?, 'pending', 0)",
		imageID).Error; err != nil {
		logger.Warnf("Failed to enqueue embed for image %d: %v", imageID, err)
		return
	}
	if err := database.DB.Exec(
		"UPDATE embed_queues SET status = 'pending', attempts = 0 "+
			"WHERE image_id = ? AND status NOT IN ('pending', 'processing')",
		imageID).Error; err != nil {
		logger.Warnf("Failed to reset embed queue row for image %d: %v", imageID, err)
		return
	}
	select {
	case EmbedWorkQueue <- imageID:
	default:
	}
}

func StartEmbeddingWorker() {
	numWorkers := config.Global.Embedding.Concurrency
	if numWorkers < 1 {
		numWorkers = 1
	}
	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			logger.Debugf("Embedding worker %d started", workerID)
			for imageID := range EmbedWorkQueue {
				func() {
					defer func() {
						if r := recover(); r != nil {
							logger.Errorf("Embedding worker %d recovered from panic on image %d: %v", workerID, imageID, r)
							// Mark permanently-failed so it isn't auto-retried every sweep.
							database.DB.Model(&models.EmbedQueue{}).
								Where("image_id = ?", imageID).
								Updates(map[string]interface{}{"status": "failed", "attempts": maxEmbedRetries + 1})
						}
					}()
					if err := ProcessEmbed(imageID); err != nil {
						logger.Warnf("Embedding worker %d failed image %d: %v", workerID, imageID, err)
					}
				}()
			}
		}(i)
	}

	// Startup: drain the persisted queue, revive anything deferred whose file
	// is now on disk, then backfill anything never scheduled.
	go func() {
		FeedPendingEmbedQueue()
		FeedDeferredEmbedQueue()
		n := EnqueueMissingEmbeddings()
		if n > 0 {
			logger.Infof("Embedding backfill enqueued %d images", n)
		}
	}()

	// Periodic sweep re-feeds pending work, retries bounded failures, revives
	// deferred rows whose file has appeared, and picks up images added while
	// running.
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			FeedPendingEmbedQueue()
			FeedRetryableFailed()
			FeedDeferredEmbedQueue()
			if n := EnqueueMissingEmbeddings(); n > 0 {
				logger.Infof("Embedding sweep enqueued %d new images", n)
			}
		}
	}()
}

// FeedPendingEmbedQueue re-pushes persisted pending rows into the worker
// channel (bounded by a time budget so it never blocks startup).
func FeedPendingEmbedQueue() {
	var ids []uint
	if err := database.DB.Table("embed_queues").
		Where("status = ?", "pending").
		Order("id ASC").Limit(4000).
		Pluck("image_id", &ids).Error; err != nil {
		logger.Warnf("FeedPendingEmbedQueue error: %v", err)
		return
	}
	for _, id := range ids {
		select {
		case EmbedWorkQueue <- id:
		case <-time.After(100 * time.Millisecond):
			return
		}
	}
}

// FeedRetryableFailed re-pushes bounded failures (attempts < maxEmbedRetries)
// so transient errors get another chance between sweeps.
func FeedRetryableFailed() {
	var ids []uint
	if err := database.DB.Table("embed_queues").
		Where("status = ? AND attempts < ?", "failed", maxEmbedRetries).
		Order("id ASC").Limit(4000).
		Pluck("image_id", &ids).Error; err != nil {
		logger.Warnf("FeedRetryableFailed error: %v", err)
		return
	}
	for _, id := range ids {
		select {
		case EmbedWorkQueue <- id:
		case <-time.After(100 * time.Millisecond):
			return
		}
	}
}

// FeedDeferredEmbedQueue revives deferred rows (images whose file was missing
// at embed time) once their file is back on disk, so a fresh-restart queue gets
// picked up as soon as images are re-downloaded. Deferred rows whose image
// record has been deleted are dropped.
func FeedDeferredEmbedQueue() {
	var ids []uint
	if err := database.DB.Table("embed_queues").
		Where("status = ?", "deferred").
		Pluck("image_id", &ids).Error; err != nil {
		logger.Warnf("FeedDeferredEmbedQueue error: %v", err)
		return
	}
	for _, id := range ids {
		if _, err := ResolveImageFilePath(id); err != nil {
			if !errors.Is(err, ErrImageFileMissing) {
				// Image row no longer exists — drop the orphaned queue row.
				database.DB.Where("image_id = ?", id).Delete(&models.EmbedQueue{})
			}
			continue
		}
		database.DB.Model(&models.EmbedQueue{}).
			Where("image_id = ? AND status = ?", id, "deferred").
			Updates(map[string]interface{}{"status": "pending", "attempts": 0})
		select {
		case EmbedWorkQueue <- id:
		default:
		}
	}
}

// EnqueueMissingEmbeddings queues every eligible image that has no embedding.
func EnqueueMissingEmbeddings() int {
	count := 0
	const chunkSize = 500
	var lastID uint
	for {
		var images []models.Image
		err := database.DB.Select("id").
			Where("type = ? AND file_exists = ? AND deleted_at IS NULL AND id > ?", "image", true, lastID).
			Where("NOT EXISTS (SELECT 1 FROM image_embeddings WHERE image_embeddings.image_id = images.id)").
			Where("NOT EXISTS (SELECT 1 FROM embed_queues WHERE embed_queues.image_id = images.id)").
			Order("id ASC").Limit(chunkSize).
			Find(&images).Error
		if err != nil {
			logger.Warnf("Embedding backfill scan error: %v", err)
			break
		}
		if len(images) == 0 {
			break
		}
		for _, im := range images {
			EnqueueEmbed(im.ID)
			count++
		}
		lastID = images[len(images)-1].ID
	}
	return count
}

// ProcessEmbed computes and stores the embedding, features and derived tags
// for one image.
func ProcessEmbed(imageID uint) error {
	attempts := markEmbedProcessing(imageID)

	path, err := ResolveImageFilePath(imageID)
	if err != nil {
		// File not on disk yet (fresh restart / awaiting re-download): park the
		// row as deferred instead of burning a retry. It is revived by
		// FeedDeferredEmbedQueue / EnqueueEmbed once the file reappears.
		if errors.Is(err, ErrImageFileMissing) {
			return deferEmbed(imageID)
		}
		return handleEmbedFailure(imageID, attempts, err)
	}

	res, err := CurrentEmbedder().Embed(path)
	if err != nil {
		return handleEmbedFailure(imageID, attempts, err)
	}

	if _, err := StoreImageEmbedding(imageID, res); err != nil {
		return handleEmbedFailure(imageID, attempts, err)
	}

	if err := UpsertDerivedTags(imageID, DeriveLowLevelTags(res.Features)); err != nil {
		logger.Warnf("Tag upsert failed for image %d: %v", imageID, err)
	}

	return markEmbedDone(imageID)
}

func markEmbedProcessing(imageID uint) int {
	var attempts int
	database.DB.Transaction(func(tx *gorm.DB) error {
		var q models.EmbedQueue
		err := tx.Where("image_id = ?", imageID).First(&q).Error
		if err != nil {
			q = models.EmbedQueue{ImageID: imageID, Status: "processing", Attempts: 1}
			if err := tx.Create(&q).Error; err != nil {
				return err
			}
			attempts = 1
			return nil
		}
		q.Status = "processing"
		q.Attempts++
		if err := tx.Save(&q).Error; err != nil {
			return err
		}
		attempts = q.Attempts
		return nil
	})
	if attempts == 0 {
		attempts = 1
	}
	return attempts
}

// deferEmbed parks an image whose file is not on disk yet (e.g. a fresh
// restart before images have been re-downloaded). It is not counted as a
// failure and gets no retry backoff; FeedDeferredEmbedQueue / EnqueueEmbed
// revive it once the file reappears.
func deferEmbed(imageID uint) error {
	database.DB.Model(&models.EmbedQueue{}).
		Where("image_id = ?", imageID).
		Updates(map[string]interface{}{"status": "deferred", "attempts": 0})
	logger.Debugf("Embed deferred for image %d: file not on disk yet", imageID)
	return nil
}

func handleEmbedFailure(imageID uint, attempts int, err error) error {
	database.DB.Model(&models.EmbedQueue{}).
		Where("image_id = ?", imageID).
		Updates(map[string]interface{}{"status": "failed", "attempts": attempts})
	if attempts < maxEmbedRetries {
		// Back off before retrying (attempt^2 * 10s, capped at 10m) so a
		// persistently failing image can't spin a worker and starve the queue.
		delay := time.Duration(attempts*attempts*10) * time.Second
		if delay > 10*time.Minute {
			delay = 10 * time.Minute
		}
		time.AfterFunc(delay, func() {
			select {
			case EmbedWorkQueue <- imageID:
			default:
			}
		})
	}
	return err
}

func markEmbedDone(imageID uint) error {
	return database.DB.Where("image_id = ?", imageID).Delete(&models.EmbedQueue{}).Error
}

// ResolveImageFilePath finds the on-disk path for an image, following the same
// gallery/source layout rules as the rest of the codebase, with a flat-layout
// fallback. No tree walks: a missing file is just a fast, cheap failure (a
// recursive scan on every retry could stall a worker for minutes).
func ResolveImageFilePath(imageID uint) (string, error) {
	var image models.Image
	if err := database.DB.Select("id, filename, source_id").First(&image, imageID).Error; err != nil {
		return "", err
	}

	sourceName := "uncategorized"
	if image.SourceID != nil {
		var src models.Source
		if err := database.DB.Select("name").First(&src, *image.SourceID).Error; err == nil {
			sourceName = src.Name
		}
	} else {
		var gallery models.Gallery
		if err := database.DB.Joins("JOIN image_galleries ON image_galleries.gallery_id = galleries.id").
			Where("image_galleries.image_id = ?", imageID).First(&gallery).Error; err == nil && gallery.SourceID != nil {
			var src models.Source
			if err := database.DB.Select("name").First(&src, *gallery.SourceID).Error; err == nil {
				sourceName = src.Name
			}
		}
	}

	candidates := []string{
		filepath.Join(UploadsDir, SanitizeDirectoryName(sourceName), filepath.Base(image.Filename)),
		filepath.Join(UploadsDir, image.Filename), // legacy flat layout
		image.Filename,                            // absolute/relative path stored directly
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("%w (image %d)", ErrImageFileMissing, imageID)
}
