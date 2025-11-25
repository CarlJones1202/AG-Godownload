package handlers

import (
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
	// Only load first image for each gallery (for thumbnail)
	if err := database.DB.Preload("Images", func(db *gorm.DB) *gorm.DB {
		return db.Limit(1).Order("created_at ASC")
	}).Find(&galleries).Error; err != nil {
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

// DeleteGallery removes a gallery and optionally its images
func DeleteGallery(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid gallery ID"})
		return
	}

	// Check if we should delete images too
	deleteImages := c.Query("delete_images") == "true"

	var gallery models.Gallery
	if err := database.DB.Preload("Images").First(&gallery, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gallery not found"})
		return
	}

	if deleteImages {
		// Delete all images in this gallery
		for _, image := range gallery.Images {
			imagePath := filepath.Join(services.UploadsDir, image.Filename)
			services.DeleteFile(imagePath)
			thumbnailPath := filepath.Join(services.UploadsDir, "thumbnails", image.Filename)
			services.DeleteFile(thumbnailPath)
		}
		// Delete image records
		database.DB.Where("gallery_id = ?", id).Delete(&models.Image{})
	}

	// Delete gallery
	if err := database.DB.Delete(&gallery).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete gallery"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Gallery deleted successfully"})
}
