package handlers

import (
	"gallery_api/database"
	"gallery_api/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

func CreateGallery(c *gin.Context) {
	var gallery models.Gallery
	if err := c.ShouldBindJSON(&gallery); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := database.DB.Create(&gallery).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create gallery"})
		return
	}

	c.JSON(http.StatusCreated, gallery)
}

func GetGalleries(c *gin.Context) {
	var galleries []models.Gallery
	if err := database.DB.Preload("Images").Find(&galleries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch galleries"})
		return
	}

	c.JSON(http.StatusOK, galleries)
}

func GetGallery(c *gin.Context) {
	id := c.Param("id")
	var gallery models.Gallery
	if err := database.DB.Preload("Images").First(&gallery, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gallery not found"})
		return
	}

	c.JSON(http.StatusOK, gallery)
}
