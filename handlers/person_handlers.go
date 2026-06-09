package handlers

import (
	"encoding/json"
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"os"
	"path/filepath"
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
		db = db.Where("LOWER(name) LIKE ? OR LOWER(aliases) LIKE ?", searchTerm, searchTerm)
	}

	var total int64
	db.Count(&total)

	var people []models.Person
	if err := db.Limit(limit).Offset(offset).Order("name ASC").Find(&people).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch people"})
		return
	}

	type PersonResponse struct {
		models.Person
		GalleryCount  int    `json:"gallery_count"`
		VideoCount    int    `json:"video_count"`
		ThumbnailPath string `json:"thumbnail_path"`
	}

	personResponses := make([]PersonResponse, len(people))
	if len(people) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"data": personResponses,
			"meta": gin.H{
				"current_page": page,
				"total_pages":  0,
				"total_items":  total,
				"limit":        limit,
			},
		})
		return
	}

	personIDs := make([]uint, len(people))
	for i, p := range people {
		personIDs[i] = p.ID
	}

	// Batch 1: Gallery counts via person_galleries
	type PersonCount struct {
		PersonID uint
		Count    int
	}
	var galleryCounts []PersonCount
	database.DB.Table("person_galleries").
		Select("person_id, COUNT(*) as count").
		Where("person_id IN ?", personIDs).
		Group("person_id").
		Scan(&galleryCounts)

	galleryCountMap := make(map[uint]int, len(people))
	for _, gc := range galleryCounts {
		galleryCountMap[gc.PersonID] = gc.Count
	}

	// Batch 2: Video counts (via galleries + direct images)
	var videoCounts []PersonCount
	database.DB.Raw(`
		SELECT person_id, COUNT(DISTINCT image_id) AS count FROM (
			SELECT pg.person_id, i.id AS image_id
			FROM person_galleries pg
			JOIN galleries g ON g.id = pg.gallery_id
			JOIN images i ON i.gallery_id = g.id
			WHERE pg.person_id IN ? AND i.type = 'video' AND i.deleted_at IS NULL AND g.deleted_at IS NULL
			UNION
			SELECT pi.person_id, pi.image_id
			FROM person_images pi
			JOIN images i ON i.id = pi.image_id
			WHERE pi.person_id IN ? AND i.type = 'video' AND i.deleted_at IS NULL
		) GROUP BY person_id
	`, personIDs, personIDs).Scan(&videoCounts)

	videoCountMap := make(map[uint]int, len(people))
	for _, vc := range videoCounts {
		videoCountMap[vc.PersonID] = vc.Count
	}

	// Batch 3: Thumbnails with fallback
	thumbnailMap := make(map[uint]string, len(people))

	// Stage 1: Parse Photos arrays (in-memory)
	noPhotoIDs := make([]uint, 0, len(people))
	for i := range people {
		if people[i].Photos != "" {
			var photos []string
			if err := json.Unmarshal([]byte(people[i].Photos), &photos); err == nil && len(photos) > 0 {
				thumbnailMap[people[i].ID] = photos[0]
				continue
			}
		}
		noPhotoIDs = append(noPhotoIDs, people[i].ID)
	}

	if len(noPhotoIDs) > 0 {
		// Stage 2: First tagged image per person
		type PersonThumb struct {
			PersonID   uint
			Filename   string
			Type       string
			SourceName string
		}
		var tagThumbs []PersonThumb
		database.DB.Raw(`
			SELECT person_id, filename, type, source_name FROM (
				SELECT pi.person_id, i.filename, i.type,
					COALESCE(s.name, 'uncategorized') AS source_name,
					ROW_NUMBER() OVER (PARTITION BY pi.person_id ORDER BY i.created_at DESC, i.id DESC) AS rn
				FROM person_images pi
				JOIN images i ON i.id = pi.image_id AND i.deleted_at IS NULL
				LEFT JOIN sources s ON s.id = i.source_id
				WHERE pi.person_id IN ?
			) ranked WHERE rn = 1
		`, noPhotoIDs).Scan(&tagThumbs)

		for _, tt := range tagThumbs {
			sanitizedSource := services.SanitizeDirectoryName(tt.SourceName)
			thumbName := filepath.Base(tt.Filename)
			if tt.Type == "video" {
				thumbName += ".jpg"
			}
			thumbnailMap[tt.PersonID] = fmt.Sprintf("/images/%s/thumbnails/%s", sanitizedSource, thumbName)
		}

		// Stage 3: First gallery image per person (fallback)
		var needsGalleryFallback []uint
		for _, pid := range noPhotoIDs {
			if _, ok := thumbnailMap[pid]; !ok {
				needsGalleryFallback = append(needsGalleryFallback, pid)
			}
		}

		if len(needsGalleryFallback) > 0 {
			var galleryThumbs []PersonThumb
			database.DB.Raw(`
				SELECT person_id, filename, type, source_name FROM (
					SELECT pg.person_id, i.filename, i.type,
						COALESCE(s.name, 'uncategorized') AS source_name,
						ROW_NUMBER() OVER (PARTITION BY pg.person_id ORDER BY g.created_at DESC, i.id ASC) AS rn
					FROM person_galleries pg
					JOIN galleries g ON g.id = pg.gallery_id AND g.deleted_at IS NULL
					JOIN images i ON i.gallery_id = g.id AND i.deleted_at IS NULL
					LEFT JOIN sources s ON s.id = g.source_id
					WHERE pg.person_id IN ?
				) ranked WHERE rn = 1
			`, needsGalleryFallback).Scan(&galleryThumbs)

			for _, gt := range galleryThumbs {
				if _, ok := thumbnailMap[gt.PersonID]; ok {
					continue
				}
				sanitizedSource := services.SanitizeDirectoryName(gt.SourceName)
				thumbName := filepath.Base(gt.Filename)
				if gt.Type == "video" {
					thumbName += ".jpg"
				}
				thumbnailMap[gt.PersonID] = fmt.Sprintf("/images/%s/thumbnails/%s", sanitizedSource, thumbName)
			}
		}
	}

	for i := range people {
		personResponses[i] = PersonResponse{
			Person:        people[i],
			GalleryCount:  galleryCountMap[people[i].ID],
			VideoCount:    videoCountMap[people[i].ID],
			ThumbnailPath: thumbnailMap[people[i].ID],
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
	if err := database.DB.Preload("Identifiers").First(&person, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	// Load galleries for this person with sorting
	sortBy := c.DefaultQuery("sort", "newest")
	seedStr := c.Query("seed")
	seed, _ := strconv.Atoi(seedStr)

	var galleries []models.Gallery
	query := database.DB.Model(&models.Gallery{}).
		Joins("JOIN person_galleries ON person_galleries.gallery_id = galleries.id").
		Where("person_galleries.person_id = ?", person.ID)

	switch sortBy {
	case "newest":
		query = query.Order("galleries.created_at DESC")
	case "oldest":
		query = query.Order("galleries.created_at ASC")
	case "shuffle":
		query = query.Order(fmt.Sprintf("(((galleries.id + 1) * 1103515245 + %d * 12345) %% 2147483647)", seed))
	default:
		query = query.Order("galleries.id DESC")
	}

	query.Find(&galleries)

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
			firstImage.WebPath = fmt.Sprintf("/images/%s/%s", sanitizedSource, filepath.Base(firstImage.Filename))

			thumbName := filepath.Base(firstImage.Filename)
			if firstImage.Type == "video" {
				thumbName += ".jpg"
				// Also mark the image type as video so the frontend knows to show the play overlay
				firstImage.Type = "video"
			}

			firstImage.ThumbnailPath = fmt.Sprintf("/images/%s/thumbnails/%s", sanitizedSource, thumbName)
			galleries[i].Images = []models.Image{firstImage}
		}

		galleryResponses[i] = GalleryResponse{
			Gallery:    galleries[i],
			ImageCount: int(count),
		}
	}

	// Get fallback thumbnail if no photos
	var thumbnailPath string
	if person.Photos != "" {
		var photos []string
		if err := json.Unmarshal([]byte(person.Photos), &photos); err == nil && len(photos) > 0 {
			thumbnailPath = photos[0]
		}
	}
	if thumbnailPath == "" {
		// Try tagged images first
		var firstImage models.Image
		err := database.DB.Model(&models.Image{}).
			Joins("JOIN person_images ON person_images.image_id = images.id").
			Where("person_images.person_id = ?", person.ID).
			Preload("Source").
			Preload("Galleries.Source").
			Order("images.created_at DESC").
			First(&firstImage).Error

		if err == nil {
			sourceName := "uncategorized"
			if firstImage.Source != nil {
				sourceName = firstImage.Source.Name
			} else if len(firstImage.Galleries) > 0 && firstImage.Galleries[0].Source != nil {
				sourceName = firstImage.Galleries[0].Source.Name
			}
			sanitizedSource := services.SanitizeDirectoryName(sourceName)
			thumbName := filepath.Base(firstImage.Filename)
			if firstImage.Type == "video" {
				thumbName += ".jpg"
			}
			thumbnailPath = fmt.Sprintf("/images/%s/thumbnails/%s", sanitizedSource, thumbName)
		}
	}
	if thumbnailPath == "" && len(galleryResponses) > 0 {
		// Fallback to first gallery icon
		if len(galleryResponses[0].Images) > 0 {
			thumbnailPath = galleryResponses[0].Images[0].ThumbnailPath
		}
	}

	// Return person with enhanced galleries
	c.JSON(http.StatusOK, gin.H{
		"id":             person.ID,
		"created_at":     person.CreatedAt,
		"updated_at":     person.UpdatedAt,
		"name":           person.Name,
		"aliases":        person.Aliases,
		"stash_id":       person.StashID,
		"birthdate":      person.Birthdate,
		"country":        person.Country,
		"ethnicity":      person.Ethnicity,
		"eye_color":      person.EyeColor,
		"hair_color":     person.HairColor,
		"height":         person.Height,
		"measurements":   person.Measurements,
		"fake_tits":      person.FakeTits,
		"career_length":  person.CareerLength,
		"tattoos":        person.Tattoos,
		"piercings":      person.Piercings,
		"bio":            person.Bio,
		"twitter":        person.Twitter,
		"instagram":      person.Instagram,
		"photos":         person.Photos,
		"thumbnail_path": thumbnailPath,
		"galleries":      galleryResponses,
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

	// Clean up photos from disk
	if person.Photos != "" {
		var photoPaths []string
		if err := json.Unmarshal([]byte(person.Photos), &photoPaths); err == nil {
			for _, webPath := range photoPaths {
				// Convert web path to system path
				// webPath: /person-images/123/hash.jpg
				// sysPath: uploads/person_images/123/hash.jpg
				relativePath := strings.TrimPrefix(webPath, "/person-images/")
				if relativePath != webPath { // cleanup only if prefix matched
					fullPath := filepath.Join(services.UploadsDir, "person_images", filepath.FromSlash(relativePath))
					services.DeleteFile(fullPath)
				}
			}
		}
	}

	// Remove the person's image directory
	personDir := filepath.Join(services.UploadsDir, "person_images", fmt.Sprintf("%d", person.ID))
	os.RemoveAll(personDir)

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

func ScanPersonFromSource(c *gin.Context) {
	id := c.Param("id")
	provider := c.Query("source")
	alias := c.Query("alias")

	if provider == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source query parameter is required (MetArt, MetartX, Playboy, PlayboyPlus, Vixen, SexArt, LifeErotic, EternalDesire, MPLStudios, VivThomas, WowGirls, or RylskyArt)"})
		return
	}

	personID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid person ID"})
		return
	}

	result, err := services.ScanSourceForPerson(uint(personID), provider, alias)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}
