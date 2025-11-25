package main

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"log"
)

// CleanupDuplicateSources removes duplicate sources, keeping the oldest one
func CleanupDuplicateSources() {
	fmt.Println("Starting duplicate source cleanup...")

	var sources []models.Source
	database.DB.Find(&sources)

	// Group sources by location
	locationMap := make(map[string][]models.Source)
	for _, source := range sources {
		locationMap[source.Location] = append(locationMap[source.Location], source)
	}

	duplicatesRemoved := 0
	for location, duplicates := range locationMap {
		if len(duplicates) > 1 {
			fmt.Printf("Found %d duplicates for: %s\n", len(duplicates), location)

			// Sort by ID (oldest first) and keep the first one
			oldest := duplicates[0]
			for i := 1; i < len(duplicates); i++ {
				if duplicates[i].ID < oldest.ID {
					oldest = duplicates[i]
				}
			}

			// Delete all except the oldest
			for _, dup := range duplicates {
				if dup.ID != oldest.ID {
					fmt.Printf("  Deleting duplicate source ID %d: %s\n", dup.ID, dup.Name)
					database.DB.Delete(&dup)
					duplicatesRemoved++
				}
			}

			fmt.Printf("  Kept source ID %d: %s\n", oldest.ID, oldest.Name)
		}
	}

	fmt.Printf("\nCleanup complete! Removed %d duplicate sources.\n", duplicatesRemoved)
}

func main() {
	// Initialize database
	database.Connect()
	database.Migrate()

	// Run cleanup
	CleanupDuplicateSources()

	log.Println("Done!")
}
