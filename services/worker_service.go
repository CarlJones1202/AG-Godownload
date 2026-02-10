package services

import (
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
)

var CrawlerQueue = make(chan uint, 100)

func AddToCrawlerQueue(sourceID uint) {
	select {
	case CrawlerQueue <- sourceID:
		logger.Debugf("Added source %d to crawler queue", sourceID)
	default:
		logger.Warn("Crawler queue full, skipping source", sourceID)
	}
}

// StartCrawlerWorker - event driven version
func StartCrawlerWorker() {
	// Startup recovery: find sources that were crawling or need crawling
	go func() {
		logger.Debug("Checking for interrupted crawls...")
		var sources []models.Source
		// Find sources that are 'crawling' (interrupted) or 'idle' but might have been missed?
		// Actually, mostly we care about 'crawling' ones that got stuck because of a crash.
		// Or maybe we just want to ensure everything is consistent.
		// For now, let's just pick up 'crawling' ones and reset them or re-queue them.
		// Also, if we want to be robust, we could pick up 'pending' if we had such a status.
		// Let's just look for 'crawling' status.
		if err := database.DB.Where("status = ?", "crawling").Find(&sources).Error; err == nil {
			logger.Infof("Found %d interrupted crawls, re-queueing...", len(sources))
			for _, s := range sources {
				AddToCrawlerQueue(s.ID)
			}
		}
	}()

	// Start workers
	numWorkers := config.Global.CrawlerWorkers
	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			logger.Debugf("Crawler worker %d started", workerID)
			for sourceID := range CrawlerQueue {
				logger.Debugf("Worker %d processing source %d", workerID, sourceID)
				if err := CrawlSource(sourceID); err != nil {
					logger.Errorf("Worker %d error crawling source %d: %v", workerID, sourceID, err)
				}
			}
		}(i)
	}
}

var AITagQueue = make(chan uint, 100)

func AddToAITagQueue(imageID uint) {
	select {
	case AITagQueue <- imageID:
		logger.Debugf("Added image %d to AI tag queue", imageID)
	default:
		logger.Warn("AI tag queue full, skipping image", imageID)
	}
}

func StartAITagWorker() {
	numWorkers := config.Global.AITagWorkers
	if numWorkers < 1 {
		numWorkers = 1
	}

	for i := 0; i < numWorkers; i++ {
		go func() {
			logger.Debug("AI Tag worker started")
			for imageID := range AITagQueue {
				logger.Debugf("AI Tag worker processing image %d", imageID)
				if err := LabelImage(imageID); err != nil {
					logger.Errorf("Error AI tagging image %d: %v", imageID, err)
				}
			}
		}()
	}

	// Startup: scan for untagged images
	go func() {
		if err := ScanUntaggedImages(); err != nil {
			logger.Errorf("Failed to scan untagged images: %v", err)
		}
	}()
}
