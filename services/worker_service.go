package services

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
)

var CrawlerQueue = make(chan uint, 100)

func AddToCrawlerQueue(sourceID uint) {
	select {
	case CrawlerQueue <- sourceID:
		fmt.Printf("Added source %d to crawler queue\n", sourceID)
	default:
		fmt.Printf("Crawler queue full, skipping source %d\n", sourceID)
	}
}

// StartCrawlerWorker - event driven version
func StartCrawlerWorker() {
	// Startup recovery: find sources that were crawling or need crawling
	go func() {
		fmt.Println("Checking for interrupted crawls...")
		var sources []models.Source
		// Find sources that are 'crawling' (interrupted) or 'idle' but might have been missed?
		// Actually, mostly we care about 'crawling' ones that got stuck because of a crash.
		// Or maybe we just want to ensure everything is consistent.
		// For now, let's just pick up 'crawling' ones and reset them or re-queue them.
		// Also, if we want to be robust, we could pick up 'pending' if we had such a status.
		// Let's just look for 'crawling' status.
		if err := database.DB.Where("status = ?", "crawling").Find(&sources).Error; err == nil {
			fmt.Printf("Found %d interrupted crawls, re-queueing...\n", len(sources))
			for _, s := range sources {
				AddToCrawlerQueue(s.ID)
			}
		}
	}()

	// Start workers
	const numWorkers = 5
	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			fmt.Printf("Crawler worker %d started\n", workerID)
			for sourceID := range CrawlerQueue {
				fmt.Printf("Worker %d processing source %d\n", workerID, sourceID)
				if err := CrawlSource(sourceID); err != nil {
					fmt.Printf("Worker %d error crawling source %d: %v\n", workerID, sourceID, err)
				}
			}
		}(i)
	}
}
