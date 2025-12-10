package handlers

import (
	"gallery_api/database"
	"gallery_api/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetPersonStats returns statistics for a person
func GetPersonStats(c *gin.Context) {
	personID := c.Param("id")

	var person models.Person
	if err := database.DB.First(&person, personID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	// Count galleries
	galleryCount := database.DB.Model(&person).Association("Galleries").Count()

	// Count images
	imageCount := database.DB.Model(&person).Association("Images").Count()

	// Count videos (images with type='video')
	var videoCount int64
	database.DB.Model(&models.Image{}).
		Joins("JOIN person_images ON person_images.image_id = images.id").
		Where("person_images.person_id = ? AND images.type = ?", personID, "video").
		Count(&videoCount)

	// Get most common tags (if images have tags)
	type TagCount struct {
		TagID uint   `json:"tag_id"`
		Name  string `json:"name"`
		Count int64  `json:"count"`
	}
	var topTags []TagCount
	database.DB.Raw(`
		SELECT tags.id as tag_id, tags.name, COUNT(*) as count
		FROM tags
		JOIN image_tags ON image_tags.tag_id = tags.id
		JOIN person_images ON person_images.image_id = image_tags.image_id
		WHERE person_images.person_id = ?
		GROUP BY tags.id, tags.name
		ORDER BY count DESC
		LIMIT 10
	`, personID).Scan(&topTags)

	stats := gin.H{
		"person_id":     person.ID,
		"person_name":   person.Name,
		"gallery_count": galleryCount,
		"image_count":   imageCount,
		"video_count":   videoCount,
		"top_tags":      topTags,
	}

	c.JSON(http.StatusOK, stats)
}
