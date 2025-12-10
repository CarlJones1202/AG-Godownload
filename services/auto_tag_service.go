package services

import (
	"encoding/json"
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"regexp"
	"strings"
	"unicode"
)

// AutoTagResult contains the results of an auto-tagging operation
type AutoTagResult struct {
	GalleriesTagged int             `json:"galleries_tagged"`
	VideosTagged    int             `json:"videos_tagged"`
	Suggestions     []TagSuggestion `json:"suggestions"`
}

// TagSuggestion represents a potential tag match
type TagSuggestion struct {
	Type       string  `json:"type"` // "gallery" or "video"
	ID         uint    `json:"id"`
	Name       string  `json:"name"`
	MatchedOn  string  `json:"matched_on"` // Which alias matched
	Confidence float64 `json:"confidence"`
}

// AutoTagPerson scans all galleries and videos for matches to person's name/aliases
func AutoTagPerson(personID uint, minConfidence float64, autoApply bool) (*AutoTagResult, error) {
	var person models.Person
	if err := database.DB.Preload("Identifiers").First(&person, personID).Error; err != nil {
		return nil, fmt.Errorf("person not found: %w", err)
	}

	// Collect search terms (name + aliases)
	searchTerms := []string{person.Name}

	// Parse aliases from JSON
	if person.Aliases != "" {
		var aliases []string
		if err := json.Unmarshal([]byte(person.Aliases), &aliases); err == nil {
			searchTerms = append(searchTerms, aliases...)
		}
	}

	// Get exclusions for this person
	var exclusions []models.PersonExclusion
	database.DB.Where("person_id = ?", personID).Find(&exclusions)

	excludedGalleries := make(map[uint]bool)
	excludedImages := make(map[uint]bool)
	for _, exc := range exclusions {
		if exc.GalleryID != nil {
			excludedGalleries[*exc.GalleryID] = true
		}
		if exc.ImageID != nil {
			excludedImages[*exc.ImageID] = true
		}
	}

	result := &AutoTagResult{
		Suggestions: []TagSuggestion{},
	}

	// Scan galleries
	var galleries []models.Gallery
	database.DB.Find(&galleries)

	for _, gallery := range galleries {
		// Skip if excluded
		if excludedGalleries[gallery.ID] {
			continue
		}

		// Check if already tagged
		var count int64
		database.DB.Table("person_galleries").
			Where("person_id = ? AND gallery_id = ?", personID, gallery.ID).
			Count(&count)
		if count > 0 {
			continue // Already tagged
		}

		// Check for matches
		for _, term := range searchTerms {
			confidence := CalculateMatchConfidence(term, gallery.Name)
			if confidence >= minConfidence {
				if autoApply {
					// Auto-apply the tag
					database.DB.Exec("INSERT INTO person_galleries (person_id, gallery_id) VALUES (?, ?)", personID, gallery.ID)
					result.GalleriesTagged++
				} else {
					// Add as suggestion
					result.Suggestions = append(result.Suggestions, TagSuggestion{
						Type:       "gallery",
						ID:         gallery.ID,
						Name:       gallery.Name,
						MatchedOn:  term,
						Confidence: confidence,
					})
				}
				break // Only match once per gallery
			}
		}
	}

	// Scan videos (images with type "video")
	var images []models.Image
	database.DB.Where("type = ?", "video").Find(&images)

	for _, image := range images {
		// Skip if excluded
		if excludedImages[image.ID] {
			continue
		}

		// Check if already tagged
		var count int64
		database.DB.Table("person_images").
			Where("person_id = ? AND image_id = ?", personID, image.ID).
			Count(&count)
		if count > 0 {
			continue // Already tagged
		}

		// Check for matches in filename
		for _, term := range searchTerms {
			confidence := CalculateMatchConfidence(term, image.Filename)
			if confidence >= minConfidence {
				if autoApply {
					// Auto-apply the tag
					database.DB.Exec("INSERT INTO person_images (person_id, image_id) VALUES (?, ?)", personID, image.ID)
					result.VideosTagged++
				} else {
					// Add as suggestion
					result.Suggestions = append(result.Suggestions, TagSuggestion{
						Type:       "video",
						ID:         image.ID,
						Name:       image.Filename,
						MatchedOn:  term,
						Confidence: confidence,
					})
				}
				break // Only match once per video
			}
		}
	}

	return result, nil
}

// CalculateMatchConfidence returns a confidence score (0.0-1.0) for a name match
func CalculateMatchConfidence(searchTerm, targetText string) float64 {
	normalizedSearch := NormalizeName(searchTerm)
	normalizedTarget := NormalizeName(targetText)

	// Exact match
	if normalizedSearch == normalizedTarget {
		return 1.0
	}

	// Whole word match (high confidence)
	wordBoundary := regexp.MustCompile(`\b` + regexp.QuoteMeta(normalizedSearch) + `\b`)
	if wordBoundary.MatchString(normalizedTarget) {
		return 0.9
	}

	// Contains as substring (medium confidence)
	if strings.Contains(normalizedTarget, normalizedSearch) {
		// Calculate ratio of match length to target length
		ratio := float64(len(normalizedSearch)) / float64(len(normalizedTarget))
		return 0.6 + (ratio * 0.2) // 0.6-0.8 range
	}

	// Fuzzy match - check if most words are present
	searchWords := strings.Fields(normalizedSearch)
	targetWords := strings.Fields(normalizedTarget)

	if len(searchWords) > 1 {
		matchedWords := 0
		for _, sw := range searchWords {
			for _, tw := range targetWords {
				if sw == tw || strings.Contains(tw, sw) || strings.Contains(sw, tw) {
					matchedWords++
					break
				}
			}
		}

		wordMatchRatio := float64(matchedWords) / float64(len(searchWords))
		if wordMatchRatio >= 0.5 {
			return 0.4 + (wordMatchRatio * 0.3) // 0.4-0.7 range
		}
	}

	return 0.0 // No match
}

// NormalizeName normalizes a name for matching
func NormalizeName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)

	// Remove special characters but keep spaces
	name = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSpace(r) {
			return r
		}
		return ' '
	}, name)

	// Collapse multiple spaces
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")

	// Trim
	name = strings.TrimSpace(name)

	return name
}
