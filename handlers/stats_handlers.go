package handlers

import (
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetDashboardStats returns aggregate counts for the dashboard
func GetDashboardStats(c *gin.Context) {
	var sourceCount int64
	database.DB.Model(&models.Source{}).Count(&sourceCount)

	var galleryCount int64
	database.DB.Model(&models.Gallery{}).Count(&galleryCount)

	var imageCount int64
	database.DB.Model(&models.Image{}).Where("type != ?", "video").Count(&imageCount)

	var videoCount int64
	database.DB.Model(&models.Image{}).Where("type = ?", "video").Count(&videoCount)

	var personCount int64
	database.DB.Model(&models.Person{}).Count(&personCount)

	c.JSON(http.StatusOK, gin.H{
		"sources":   sourceCount,
		"galleries": galleryCount,
		"images":    imageCount,
		"videos":    videoCount,
		"people":    personCount,
		"downloads": services.GetGlobalDownloadStatus(),
	})
}

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
	// Count images: Sum of images in galleries linked to this person
	// We want to count ALL images in galleries that this person is in, not just images directly tagged with this person
	var imageCount int64
	database.DB.Model(&models.Image{}).
		Joins("JOIN person_galleries ON person_galleries.gallery_id = images.gallery_id").
		Where("person_galleries.person_id = ? AND images.type != ?", personID, "video").
		Count(&imageCount)

	// Count video galleries: Count distinct galleries that contain videos and are linked to this person
	var videoCount int64
	database.DB.Model(&models.Gallery{}).
		Joins("JOIN person_galleries ON person_galleries.gallery_id = galleries.id").
		Joins("JOIN images ON images.gallery_id = galleries.id").
		Where("person_galleries.person_id = ? AND images.type = ?", personID, "video").
		Distinct("galleries.id").
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
