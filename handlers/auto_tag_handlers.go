package handlers

import (
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// AutoTagPerson triggers auto-tagging for a person based on their identifiers
func AutoTagPerson(c *gin.Context) {
	personID := c.Param("id")

	// Parse query parameters
	minConfidence := 0.6 // Default
	if conf := c.Query("minConfidence"); conf != "" {
		if parsed, err := strconv.ParseFloat(conf, 64); err == nil {
			minConfidence = parsed
		}
	}

	autoApply := c.Query("autoApply") == "true"

	// Convert personID to uint
	id, err := strconv.ParseUint(personID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid person ID"})
		return
	}

	// Run auto-tagging
	result, err := services.AutoTagPerson(uint(id), minConfidence, autoApply)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ExcludeGalleryFromPerson marks a gallery as excluded for a person
func ExcludeGalleryFromPerson(c *gin.Context) {
	personID := c.Param("id")
	galleryID := c.Param("galleryId")

	// Convert IDs
	pid, err := strconv.ParseUint(personID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid person ID"})
		return
	}

	gid, err := strconv.ParseUint(galleryID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid gallery ID"})
		return
	}

	galleryIDUint := uint(gid)

	// Create exclusion
	exclusion := models.PersonExclusion{
		PersonID:  uint(pid),
		GalleryID: &galleryIDUint,
	}

	if err := database.DB.Create(&exclusion).Error; err != nil {
		// Check if already exists
		if database.DB.Where("person_id = ? AND gallery_id = ?", pid, gid).First(&models.PersonExclusion{}).Error == nil {
			c.JSON(http.StatusOK, gin.H{"message": "Already excluded"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create exclusion"})
		return
	}

	// Remove existing tag if present
	database.DB.Exec("DELETE FROM person_galleries WHERE person_id = ? AND gallery_id = ?", pid, gid)

	c.JSON(http.StatusOK, gin.H{"message": "Gallery excluded and untagged"})
}

// ExcludeVideoFromPerson marks a video as excluded for a person
func ExcludeVideoFromPerson(c *gin.Context) {
	personID := c.Param("id")
	imageID := c.Param("imageId")

	// Convert IDs
	pid, err := strconv.ParseUint(personID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid person ID"})
		return
	}

	iid, err := strconv.ParseUint(imageID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid image ID"})
		return
	}

	imageIDUint := uint(iid)

	// Create exclusion
	exclusion := models.PersonExclusion{
		PersonID: uint(pid),
		ImageID:  &imageIDUint,
	}

	if err := database.DB.Create(&exclusion).Error; err != nil {
		// Check if already exists
		if database.DB.Where("person_id = ? AND image_id = ?", pid, iid).First(&models.PersonExclusion{}).Error == nil {
			c.JSON(http.StatusOK, gin.H{"message": "Already excluded"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create exclusion"})
		return
	}

	// Remove existing tag if present
	database.DB.Exec("DELETE FROM person_images WHERE person_id = ? AND image_id = ?", pid, iid)

	c.JSON(http.StatusOK, gin.H{"message": "Video excluded and untagged"})
}

// RemoveExclusion removes an exclusion, allowing content to be tagged again
func RemoveExclusion(c *gin.Context) {
	personID := c.Param("id")
	exclusionID := c.Param("exclusionId")

	// Convert IDs
	pid, err := strconv.ParseUint(personID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid person ID"})
		return
	}

	eid, err := strconv.ParseUint(exclusionID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid exclusion ID"})
		return
	}

	// Verify exclusion belongs to this person
	var exclusion models.PersonExclusion
	if err := database.DB.First(&exclusion, eid).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Exclusion not found"})
		return
	}

	if exclusion.PersonID != uint(pid) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Exclusion does not belong to this person"})
		return
	}

	// Delete exclusion
	if err := database.DB.Delete(&exclusion).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove exclusion"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Exclusion removed"})
}

// GetPersonExclusions returns all exclusions for a person
func GetPersonExclusions(c *gin.Context) {
	personID := c.Param("id")

	var exclusions []models.PersonExclusion
	if err := database.DB.Where("person_id = ?", personID).Find(&exclusions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch exclusions"})
		return
	}

	// Enrich with gallery/image names
	type EnrichedExclusion struct {
		models.PersonExclusion
		GalleryName string `json:"gallery_name,omitempty"`
		ImageName   string `json:"image_name,omitempty"`
	}

	var enriched []EnrichedExclusion
	for _, exc := range exclusions {
		e := EnrichedExclusion{PersonExclusion: exc}

		if exc.GalleryID != nil {
			var gallery models.Gallery
			if database.DB.First(&gallery, *exc.GalleryID).Error == nil {
				e.GalleryName = gallery.Name
			}
		}

		if exc.ImageID != nil {
			var image models.Image
			if database.DB.First(&image, *exc.ImageID).Error == nil {
				e.ImageName = image.Filename
			}
		}

		enriched = append(enriched, e)
	}

	c.JSON(http.StatusOK, gin.H{"exclusions": enriched})
}

// ApplyAutoTagSuggestions applies selected auto-tag suggestions
func ApplyAutoTagSuggestions(c *gin.Context) {
	personID := c.Param("id")

	var req struct {
		Suggestions []struct {
			Type string `json:"type"`
			ID   uint   `json:"id"`
		} `json:"suggestions"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pid, err := strconv.ParseUint(personID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid person ID"})
		return
	}

	galleriesTagged := 0
	videosTagged := 0

	for _, sugg := range req.Suggestions {
		if sugg.Type == "gallery" {
			// Check not excluded
			var count int64
			database.DB.Model(&models.PersonExclusion{}).
				Where("person_id = ? AND gallery_id = ?", pid, sugg.ID).
				Count(&count)
			if count > 0 {
				continue // Excluded
			}

			// Add tag
			database.DB.Exec("INSERT OR IGNORE INTO person_galleries (person_id, gallery_id) VALUES (?, ?)", pid, sugg.ID)
			galleriesTagged++
		} else if sugg.Type == "video" {
			// Check not excluded
			var count int64
			database.DB.Model(&models.PersonExclusion{}).
				Where("person_id = ? AND image_id = ?", pid, sugg.ID).
				Count(&count)
			if count > 0 {
				continue // Excluded
			}

			// Add tag
			database.DB.Exec("INSERT OR IGNORE INTO person_images (person_id, image_id) VALUES (?, ?)", pid, sugg.ID)
			videosTagged++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"galleries_tagged": galleriesTagged,
		"videos_tagged":    videosTagged,
	})
}
