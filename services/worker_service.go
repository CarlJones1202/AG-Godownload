package services

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"time"
)

// StartCrawlerWorker starts a background goroutine that periodically checks for sources to crawl
func StartCrawlerWorker(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		fmt.Println("Crawler worker started")

		for range ticker.C {
			fmt.Println("Checking for sources to crawl...")

			// Find sources that haven't been checked yet or haven't been checked recently
			var sources []models.Source

			// Get sources that are idle and either never checked or checked more than 1 hour ago
			oneHourAgo := time.Now().Add(-1 * time.Hour)
			database.DB.Where("status = ? AND (last_checked_at IS NULL OR last_checked_at < ?)", "idle", oneHourAgo).
				Or("status = ?", "").
				Find(&sources)

			if len(sources) == 0 {
				fmt.Println("No sources to crawl")
				continue
			}

			fmt.Printf("Found %d sources to crawl\n", len(sources))

			// Crawl each source
			for _, source := range sources {
				fmt.Printf("Crawling source %d: %s\n", source.ID, source.Name)

				// Crawl in a separate goroutine to avoid blocking
				go func(sourceID uint) {
					if err := CrawlSource(sourceID); err != nil {
						fmt.Printf("Error crawling source %d: %v\n", sourceID, err)
					}
				}(source.ID)
			}
		}
	}()
}
