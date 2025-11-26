package handlers

import (
	"gallery_api/database"
	"gallery_api/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

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
