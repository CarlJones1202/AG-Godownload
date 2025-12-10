package handlers

import (
	"gallery_api/database"
	"gallery_api/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// LinkGalleryToPerson manually links a gallery to a person
func LinkGalleryToPerson(c *gin.Context) {
	personID := c.Param("id")
	galleryID := c.Param("galleryId")

	var person models.Person
	if err := database.DB.First(&person, personID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	var gallery models.Gallery
	if err := database.DB.First(&gallery, galleryID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gallery not found"})
		return
	}

	// Add the association (GORM will handle duplicates)
	if err := database.DB.Model(&person).Association("Galleries").Append(&gallery); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to link gallery"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Gallery linked successfully"})
}

// UnlinkGalleryFromPerson removes a specific gallery from a person
func UnlinkGalleryFromPerson(c *gin.Context) {
	personID := c.Param("id")
	galleryID := c.Param("galleryId")

	var person models.Person
	if err := database.DB.First(&person, personID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	var gallery models.Gallery
	if err := database.DB.First(&gallery, galleryID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gallery not found"})
		return
	}

	// Remove the association
	if err := database.DB.Model(&person).Association("Galleries").Delete(&gallery); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to unlink gallery"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Gallery unlinked successfully"})
}

// LinkImageToPerson manually links an image/video to a person
func LinkImageToPerson(c *gin.Context) {
	personID := c.Param("id")
	imageID := c.Param("imageId")

	var person models.Person
	if err := database.DB.First(&person, personID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	var image models.Image
	if err := database.DB.First(&image, imageID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
		return
	}

	// Add the association (GORM will handle duplicates)
	if err := database.DB.Model(&person).Association("Images").Append(&image); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to link image"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Image linked successfully"})
}

// UnlinkImageFromPerson removes a specific image/video from a person
func UnlinkImageFromPerson(c *gin.Context) {
	personID := c.Param("id")
	imageID := c.Param("imageId")

	var person models.Person
	if err := database.DB.First(&person, personID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	var image models.Image
	if err := database.DB.First(&image, imageID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
		return
	}

	// Remove the association
	if err := database.DB.Model(&person).Association("Images").Delete(&image); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to unlink image"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Image unlinked successfully"})
}
