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

	// Batch-load already-tagged links into memory
	type galleryLink struct{ GalleryID uint }
	var existingGalleries []galleryLink
	database.DB.Table("person_galleries").
		Select("gallery_id").
		Where("person_id = ?", personID).
		Scan(&existingGalleries)

	alreadyTaggedGallery := make(map[uint]bool, len(existingGalleries))
	for _, link := range existingGalleries {
		alreadyTaggedGallery[link.GalleryID] = true
	}

	type imageLink struct{ ImageID uint }
	var existingImages []imageLink
	database.DB.Table("person_images").
		Select("image_id").
		Where("person_id = ?", personID).
		Scan(&existingImages)

	alreadyTaggedImage := make(map[uint]bool, len(existingImages))
	for _, link := range existingImages {
		alreadyTaggedImage[link.ImageID] = true
	}

	// Collect batch inserts for auto-apply
	type galleryInsert struct {
		PersonID  uint
		GalleryID uint
	}
	var galleryInserts []galleryInsert

	type imageInsert struct {
		PersonID uint
		ImageID  uint
	}
	var imageInserts []imageInsert

	// Chunked gallery scan
	const chunkSize = 500
	offset := 0
	for {
		var galleries []models.Gallery
		if err := database.DB.Select("id, name").Limit(chunkSize).Offset(offset).Find(&galleries).Error; err != nil {
			break
		}
		if len(galleries) == 0 {
			break
		}

		for _, gallery := range galleries {
			if excludedGalleries[gallery.ID] || alreadyTaggedGallery[gallery.ID] {
				continue
			}

			for _, term := range searchTerms {
				confidence := CalculateMatchConfidence(term, gallery.Name)
				if confidence >= minConfidence {
					if autoApply {
						galleryInserts = append(galleryInserts, galleryInsert{personID, gallery.ID})
						result.GalleriesTagged++
					} else {
						result.Suggestions = append(result.Suggestions, TagSuggestion{
							Type: "gallery", ID: gallery.ID, Name: gallery.Name,
							MatchedOn: term, Confidence: confidence,
						})
					}
					break
				}
			}
		}
		offset += chunkSize
	}

	// Batch insert gallery tags in a single transaction
	if len(galleryInserts) > 0 {
		tx := database.DB.Begin()
		for _, ins := range galleryInserts {
			if err := tx.Exec("INSERT INTO person_galleries (person_id, gallery_id) VALUES (?, ?)", ins.PersonID, ins.GalleryID).Error; err != nil {
				tx.Rollback()
				break
			}
		}
		tx.Commit()
	}

	// Chunked video scan
	offset = 0
	for {
		var images []models.Image
		if err := database.DB.Select("id, filename").Where("type = ?", "video").Limit(chunkSize).Offset(offset).Find(&images).Error; err != nil {
			break
		}
		if len(images) == 0 {
			break
		}

		for _, image := range images {
			if excludedImages[image.ID] || alreadyTaggedImage[image.ID] {
				continue
			}

			for _, term := range searchTerms {
				confidence := CalculateMatchConfidence(term, image.Filename)
				if confidence >= minConfidence {
					if autoApply {
						imageInserts = append(imageInserts, imageInsert{personID, image.ID})
						result.VideosTagged++
					} else {
						result.Suggestions = append(result.Suggestions, TagSuggestion{
							Type: "video", ID: image.ID, Name: image.Filename,
							MatchedOn: term, Confidence: confidence,
						})
					}
					break
				}
			}
		}
		offset += chunkSize
	}

	// Batch insert video tags in a single transaction
	if len(imageInserts) > 0 {
		tx := database.DB.Begin()
		for _, ins := range imageInserts {
			if err := tx.Exec("INSERT INTO person_images (person_id, image_id) VALUES (?, ?)", ins.PersonID, ins.ImageID).Error; err != nil {
				tx.Rollback()
				break
			}
		}
		tx.Commit()
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
