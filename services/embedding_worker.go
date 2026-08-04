package services

import (
	"fmt"
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"io"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"
)

const maxEmbedRetries = 4

// EmbedWorkQueue feeds embedding workers. The embed_queues table is the source
// of truth for pending work across restarts; dropped channel sends are picked
// up again by the startup and periodic sweeps.
var EmbedWorkQueue = make(chan uint, 1024)

// EnqueueEmbed schedules an image for embedding + tagging. Idempotent.
func EnqueueEmbed(imageID uint) {
	if err := database.DB.Exec(
		"INSERT OR IGNORE INTO embed_queues (image_id, status, attempts) VALUES (?, 'pending', 0)",
		imageID).Error; err != nil {
		logger.Warnf("Failed to enqueue embed for image %d: %v", imageID, err)
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
				if err := ProcessEmbed(imageID); err != nil {
					logger.Warnf("Embedding worker %d failed image %d: %v", workerID, imageID, err)
				}
			}
		}(i)
	}

	// Startup: drain the persisted queue, then backfill anything unindexed.
	go func() {
		FeedPendingEmbedQueue()
		n := EnqueueMissingEmbeddings()
		if n > 0 {
			logger.Infof("Embedding backfill enqueued %d images", n)
		}
	}()

	// Periodic sweep covers images added while running / failed retries.
	go func() {
		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			FeedPendingEmbedQueue()
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

func handleEmbedFailure(imageID uint, attempts int, err error) error {
	database.DB.Model(&models.EmbedQueue{}).
		Where("image_id = ?", imageID).
		Updates(map[string]interface{}{"status": "failed", "attempts": attempts})
	if attempts < maxEmbedRetries {
		select {
		case EmbedWorkQueue <- imageID:
		default:
		}
	}
	return err
}

func markEmbedDone(imageID uint) error {
	return database.DB.Where("image_id = ?", imageID).Delete(&models.EmbedQueue{}).Error
}

// ResolveImageFilePath finds the on-disk path for an image, following the same
// gallery/source layout rules as the rest of the codebase, with a recursive
// walk fallback for legacy flat layouts.
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

	baseName := filepath.Base(image.Filename)
	fullPath := filepath.Join(UploadsDir, SanitizeDirectoryName(sourceName), baseName)
	if _, err := os.Stat(fullPath); err == nil {
		return fullPath, nil
	}
	for _, candidate := range []string{filepath.Join(UploadsDir, image.Filename), image.Filename} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	found := ""
	walkErr := filepath.Walk(UploadsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && (info.Name() == baseName || info.Name() == image.Filename) {
			found = path
			return io.EOF
		}
		return nil
	})
	if walkErr != nil && walkErr != io.EOF {
		return "", fmt.Errorf("recursive search failed: %w", walkErr)
	}
	if found != "" {
		return found, nil
	}
	return "", fmt.Errorf("image file not found on disk (image %d)", imageID)
}