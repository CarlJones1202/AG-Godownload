package main

import (
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"strings"
	"unicode"
)

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' || r == '’'
}

func cleanPrefixDelimiters(s string) string {
	for {
		original := s
		s = strings.TrimSpace(s)
		// Trim leading hyphens, en-dashes, em-dashes, colons, semicolons
		s = strings.TrimLeft(s, "-–—:;")
		s = strings.TrimSpace(s)
		if s == original {
			break
		}
	}
	return s
}

func cleanupGalleryNames() {
	logger.Info("Starting gallery name cleanup...")

	var galleries []models.Gallery
	// Preload associated People
	if err := database.DB.Preload("People").Find(&galleries).Error; err != nil {
		logger.Fatalf("Failed to fetch galleries: %v", err)
	}

	updatedCount := 0

	for _, gallery := range galleries {
		if len(gallery.People) == 0 {
			continue
		}

		originalName := gallery.Name
		currentName := originalName

		for _, person := range gallery.People {
			if person.Name == "" {
				continue
			}

			pNameLower := strings.ToLower(person.Name)
			gNameLower := strings.ToLower(currentName)

			if strings.HasPrefix(gNameLower, pNameLower) {
				runes := []rune(currentName)
				pRunes := []rune(person.Name)

				// Check boundary to avoid matching "Bob" in "Bobby" or "Bob's"
				if len(runes) > len(pRunes) {
					nextRune := runes[len(pRunes)]
					if isWordChar(nextRune) {
						// Not a boundary, skip this person
						continue
					}
				} else if len(runes) == len(pRunes) {
					// The gallery name is exactly the person's name.
					// Stripping would leave it empty, so we skip.
					continue
				}

				// Strip prefix
				trimmed := string(runes[len(pRunes):])
				cleaned := cleanPrefixDelimiters(trimmed)

				if cleaned != "" && cleaned != currentName {
					logger.Infof("Suggested rename for Gallery ID %d: '%s' -> '%s' (based on person: %s)", gallery.ID, currentName, cleaned, person.Name)
					currentName = cleaned
				}
			}
		}

		if currentName != originalName {
			// Update the database
			gallery.Name = currentName
			if err := database.DB.Save(&gallery).Error; err != nil {
				logger.Errorf("Failed to update gallery ID %d: %v", gallery.ID, err)
			} else {
				logger.Infof("Successfully updated Gallery ID %d to '%s'", gallery.ID, currentName)
				updatedCount++
			}
		}
	}

	logger.Infof("Gallery name cleanup complete! Updated %d galleries.", updatedCount)
}

func main() {
	// Load Configuration
	config.Load()

	// Configure logger from config
	logger.SetLevelFromString(config.Global.LogLevel)

	// Initialize Database
	logger.Info("Connecting to database...")
	database.Connect(config.Global.DatabasePath)

	// Run cleanup
	cleanupGalleryNames()
}
