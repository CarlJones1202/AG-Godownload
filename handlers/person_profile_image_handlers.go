package handlers

import (
	"gallery_api/database"
	"gallery_api/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetPersonImages lists images related to a person: images from their linked
// galleries plus any directly linked images, newest first. Used by the profile
// picture picker.
func GetPersonImages(c *gin.Context) {
	personID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid person ID"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "2000"))
	if limit < 1 {
		limit = 2000
	}
	if limit > 10000 {
		limit = 10000
	}

	var images []models.Image
	if err := database.DB.
		Where("type = 'image'").
		Where("id IN (SELECT ig.image_id FROM image_galleries ig JOIN person_galleries pg ON pg.gallery_id = ig.gallery_id WHERE pg.person_id = ? "+
			"UNION SELECT image_id FROM person_images WHERE person_id = ?)", uint(personID), uint(personID)).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&images).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch person images"})
		return
	}

	populateImagePaths(images)

	c.JSON(http.StatusOK, gin.H{"data": images})
}

// SetPersonProfileImage sets a person's profile picture to a chosen gallery image.
func SetPersonProfileImage(c *gin.Context) {
	personID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid person ID"})
		return
	}

	var req struct {
		ImageID uint `json:"image_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var person models.Person
	if err := database.DB.First(&person, personID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	var image models.Image
	if err := database.DB.First(&image, req.ImageID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
		return
	}
	if image.Type == "video" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Videos cannot be used as a profile picture"})
		return
	}

	person.ProfileImageID = &req.ImageID
	if err := database.DB.Save(&person).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update person"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Profile picture set", "person": person})
}

// ClearPersonProfileImage removes the user-chosen profile picture, reverting to
// the default photo/thumbnail fallback.
func ClearPersonProfileImage(c *gin.Context) {
	personID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid person ID"})
		return
	}

	var person models.Person
	if err := database.DB.First(&person, personID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	person.ProfileImageID = nil
	if err := database.DB.Save(&person).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update person"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Profile picture cleared", "person": person})
}

// personProfileThumbnailPath resolves the display path for a person's chosen
// profile image (the gallery image thumbnail), returning "" when unset/missing.
func personProfileThumbnailPath(person *models.Person) string {
	if person.ProfileImageID == nil {
		return ""
	}
	var image models.Image
	if err := database.DB.First(&image, *person.ProfileImageID).Error; err != nil {
		return ""
	}
	p := []models.Image{image}
	populateImagePaths(p)
	return p[0].ThumbnailPath
}
