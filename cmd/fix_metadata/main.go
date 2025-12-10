package main

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/services"
	"log"
)

func main() {
	fmt.Println("Starting Metadata and Title Repair Tool...")

	// Initialize Logger
	logger.SetLevel(logger.INFO)

	// Initialize Database
	fmt.Println("Connecting to database...")
	database.Connect("gallery.db")

	// Run the scan
	fmt.Println("Scanning ALL videos for metadata updates (Force Mode)...")
	if err := services.ScanMissingMetadata(database.DB, true); err != nil {
		log.Fatalf("Error during scan: %v", err)
	}

	fmt.Println("Repair complete! You can now restart the main server.")
}
