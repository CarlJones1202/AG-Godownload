package services

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
)

// RecoverInterruptedCrawls resets sources that were crawling when the server stopped
func RecoverInterruptedCrawls() {
	fmt.Println("Checking for interrupted crawls...")

	var crawlingSources []models.Source
	database.DB.Where("status = ?", "crawling").Find(&crawlingSources)

	if len(crawlingSources) == 0 {
		fmt.Println("No interrupted crawls found")
		return
	}

	fmt.Printf("Found %d interrupted crawls, resetting to idle...\n", len(crawlingSources))

	for _, source := range crawlingSources {
		fmt.Printf("Resetting source %d: %s\n", source.ID, source.Name)
		database.DB.Model(&source).Update("Status", "idle")
	}

	fmt.Println("Interrupted crawls recovered")
}
