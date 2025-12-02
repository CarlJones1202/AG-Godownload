package handlers

import (
	"encoding/json"
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func CreatePerson(c *gin.Context) {
	var req struct {
		Name    string   `json:"name" binding:"required"`
		Aliases []string `json:"aliases"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Convert aliases array to JSON string
	aliasesJSON, _ := json.Marshal(req.Aliases)

	person := models.Person{
		Name:    req.Name,
		Aliases: string(aliasesJSON),
	}

	if err := database.DB.Create(&person).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create person"})
		return
	}

	c.JSON(http.StatusCreated, person)
}

func GetPeople(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	offset := (page - 1) * limit

	query := c.Query("q")

	db := database.DB.Model(&models.Person{})
	if query != "" {
		searchTerm := "%" + strings.ToLower(query) + "%"
		db = db.Where("LOWER(name) LIKE ?", searchTerm)
	}

	var total int64
	db.Count(&total)

	var people []models.Person
	if err := db.Limit(limit).Offset(offset).Order("name ASC").Find(&people).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch people"})
		return
	}

	// Create response with gallery counts
	type PersonResponse struct {
		models.Person
		GalleryCount  int    `json:"gallery_count"`
		ThumbnailPath string `json:"thumbnail_path"`
	}

	personResponses := make([]PersonResponse, len(people))
	for i := range people {
		// Get gallery count
		var count int64
		database.DB.Model(&models.Gallery{}).
			Joins("JOIN person_galleries ON person_galleries.gallery_id = galleries.id").
			Where("person_galleries.person_id = ?", people[i].ID).
			Count(&count)

		// Get first image for thumbnail
		var thumbnailPath string

		// First, try to get from photos array
		if people[i].Photos != "" {
			var photos []string
			if err := json.Unmarshal([]byte(people[i].Photos), &photos); err == nil && len(photos) > 0 {
				thumbnailPath = photos[0]
			}
		}

		// If no photo in photos array, try to get from first gallery
		if thumbnailPath == "" {
			var firstGallery models.Gallery
			// Find the first gallery for this person that has images
			err := database.DB.Model(&models.Gallery{}).
				Joins("JOIN person_galleries ON person_galleries.gallery_id = galleries.id").
				Where("person_galleries.person_id = ?", people[i].ID).
				Preload("Source").
				First(&firstGallery).Error

			if err == nil {
				var firstImage models.Image
				if err := database.DB.Where("gallery_id = ?", firstGallery.ID).Order("created_at ASC").First(&firstImage).Error; err == nil {
					// Determine source name
					sourceName := "uncategorized"
					if firstGallery.Source != nil {
						sourceName = firstGallery.Source.Name
					} else if firstGallery.SourceID != nil {
						var source models.Source
						if err := database.DB.First(&source, *firstGallery.SourceID).Error; err == nil {
							sourceName = source.Name
						}
					}

					sanitizedSource := services.SanitizeDirectoryName(sourceName)
					thumbnailPath = fmt.Sprintf("/images/%s/thumbnails/%s", sanitizedSource, firstImage.Filename)
				}
			}
		}

		personResponses[i] = PersonResponse{
			Person:        people[i],
			GalleryCount:  int(count),
			ThumbnailPath: thumbnailPath,
		}
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))

	c.JSON(http.StatusOK, gin.H{
		"data": personResponses,
		"meta": gin.H{
			"current_page": page,
			"total_pages":  totalPages,
			"total_items":  total,
			"limit":        limit,
		},
	})
}

func GetPerson(c *gin.Context) {
	id := c.Param("id")
	var person models.Person
	if err := database.DB.First(&person, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	// Load galleries for this person
	var galleries []models.Gallery
	database.DB.Model(&person).Association("Galleries").Find(&galleries)

	// Enhance galleries with image counts and first image
	type GalleryResponse struct {
		models.Gallery
		ImageCount int `json:"image_count"`
	}

	galleryResponses := make([]GalleryResponse, len(galleries))
	for i := range galleries {
		// Preload Source for path generation
		database.DB.Preload("Source").First(&galleries[i], galleries[i].ID)

		// Get image count
		var count int64
		database.DB.Model(&models.Image{}).Where("gallery_id = ?", galleries[i].ID).Count(&count)

		// Load first image for thumbnail
		var firstImage models.Image
		if err := database.DB.Where("gallery_id = ?", galleries[i].ID).Order("created_at ASC").First(&firstImage).Error; err == nil {
			// Determine source name
			sourceName := "uncategorized"
			if galleries[i].Source != nil {
				sourceName = galleries[i].Source.Name
			} else if galleries[i].SourceID != nil {
				var source models.Source
				if err := database.DB.First(&source, *galleries[i].SourceID).Error; err == nil {
					sourceName = source.Name
				}
			}

			sanitizedSource := services.SanitizeDirectoryName(sourceName)
			firstImage.WebPath = fmt.Sprintf("/images/%s/%s", sanitizedSource, firstImage.Filename)
			firstImage.ThumbnailPath = fmt.Sprintf("/images/%s/thumbnails/%s", sanitizedSource, firstImage.Filename)
			galleries[i].Images = []models.Image{firstImage}
		}

		galleryResponses[i] = GalleryResponse{
			Gallery:    galleries[i],
			ImageCount: int(count),
		}
	}

	// Return person with enhanced galleries
	c.JSON(http.StatusOK, gin.H{
		"id":            person.ID,
		"created_at":    person.CreatedAt,
		"updated_at":    person.UpdatedAt,
		"name":          person.Name,
		"aliases":       person.Aliases,
		"stash_id":      person.StashID,
		"birthdate":     person.Birthdate,
		"country":       person.Country,
		"ethnicity":     person.Ethnicity,
		"eye_color":     person.EyeColor,
		"hair_color":    person.HairColor,
		"height":        person.Height,
		"measurements":  person.Measurements,
		"fake_tits":     person.FakeTits,
		"career_length": person.CareerLength,
		"tattoos":       person.Tattoos,
		"piercings":     person.Piercings,
		"bio":           person.Bio,
		"twitter":       person.Twitter,
		"instagram":     person.Instagram,
		"photos":        person.Photos,
		"galleries":     galleryResponses,
	})
}

func UpdatePerson(c *gin.Context) {
	id := c.Param("id")
	var person models.Person
	if err := database.DB.First(&person, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	var req struct {
		Name    string   `json:"name"`
		Aliases []string `json:"aliases"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != "" {
		person.Name = req.Name
	}
	if req.Aliases != nil {
		aliasesJSON, _ := json.Marshal(req.Aliases)
		person.Aliases = string(aliasesJSON)
	}

	if err := database.DB.Save(&person).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update person"})
		return
	}

	c.JSON(http.StatusOK, person)
}

func DeletePerson(c *gin.Context) {
	id := c.Param("id")
	var person models.Person
	if err := database.DB.First(&person, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	if err := database.DB.Delete(&person).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete person"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Person deleted successfully"})
}

func LinkPersonToGalleries(c *gin.Context) {
	id := c.Param("id")
	var person models.Person
	if err := database.DB.First(&person, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	// Parse aliases from JSON
	var aliases []string
	if person.Aliases != "" {
		json.Unmarshal([]byte(person.Aliases), &aliases)
	}

	// Build search terms (name + aliases)
	baseTerms := append([]string{person.Name}, aliases...)
	var searchTerms []string

	for _, term := range baseTerms {
		term = strings.ToLower(term)
		searchTerms = append(searchTerms, term)

		// Add variation with hyphens instead of spaces
		if strings.Contains(term, " ") {
			searchTerms = append(searchTerms, strings.ReplaceAll(term, " ", "-"))
			searchTerms = append(searchTerms, strings.ReplaceAll(term, " ", "%20"))
		}
	}

	// Find galleries with sources that match any search term
	var galleries []models.Gallery
	database.DB.
		Joins("LEFT JOIN sources ON sources.id = galleries.source_id").
		Where("sources.location IS NOT NULL").
		Find(&galleries)

	var matchedGalleries []*models.Gallery
	for i := range galleries {
		// Get the source location
		var source models.Source
		if galleries[i].SourceID != nil {
			database.DB.First(&source, *galleries[i].SourceID)
			locationLower := strings.ToLower(source.Location)

			// Check if any search term is in the location
			for _, term := range searchTerms {
				if strings.Contains(locationLower, term) {
					matchedGalleries = append(matchedGalleries, &galleries[i])
					break
				}
			}
		}
	}

	// Link matched galleries to person
	if len(matchedGalleries) > 0 {
		database.DB.Model(&person).Association("Galleries").Replace(matchedGalleries)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":           "Galleries linked successfully",
		"matched_count":     len(matchedGalleries),
		"matched_galleries": matchedGalleries,
	})
}
