package main

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/services"
	"log"
)

func main() {
	fmt.Println("Starting Provider Thumbnail Fixer...")

	logger.SetLevel(logger.INFO)

	fmt.Println("Connecting to database...")
	database.Connect("gallery.db")

	// Run validation which will attempt to re-download or clear invalid thumbnails
	valid, fixed, err := services.ValidateProviderThumbnails()
	if err != nil {
		log.Fatalf("Provider thumbnail validation failed: %v", err)
	}

	fmt.Printf("Validation complete: %d valid, %d fixed/updated\n", valid, fixed)
}
