package services

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"time"
)

// StartCrawlerWorker - optimized version
func StartCrawlerWorker(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		fmt.Println("Crawler worker started")

		for range ticker.C {
			fmt.Println("Checking for sources to crawl...")

			var sources []models.Source
			oneHourAgo := time.Now().Add(-1 * time.Hour)

			// Use explicit conditions + proper indexing
			err := database.DB.
				Where("(status = ? AND (last_checked_at IS NULL OR last_checked_at < ?)) AND deleted_at IS NULL", "idle", oneHourAgo).
				Or("status = '' AND deleted_at IS NULL").
				Order("last_checked_at ASC NULLS FIRST"). // important: process oldest first
				Limit(50).                                // CRITICAL: don't try to crawl 10k sources at once!
				Find(&sources).Error

			if err != nil {
				fmt.Printf("Error querying sources: %v\n", err)
				continue
			}

			if len(sources) == 0 {
				fmt.Println("No sources to crawl")
				continue
			}

			fmt.Printf("Found %d sources to crawl\n", len(sources))

			for _, source := range sources {
				go func(s models.Source) {
					if err := CrawlSource(s.ID); err != nil {
						fmt.Printf("Error crawling source %d (%s): %v\n", s.ID, s.Name, err)
					}
				}(source) // pass by value!
			}
		}
	}()
}
