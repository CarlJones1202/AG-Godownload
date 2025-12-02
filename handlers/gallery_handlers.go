package handlers

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

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
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "12"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 12
	}
	offset := (page - 1) * limit

	var total int64
	database.DB.Model(&models.Gallery{}).Count(&total)

	var galleries []models.Gallery
	// Load galleries with Source preloaded
	if err := database.DB.Preload("Source").Limit(limit).Offset(offset).Find(&galleries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch galleries"})
		return
	}

	// Create response with image counts and first image
	type GalleryResponse struct {
		models.Gallery
		ImageCount int `json:"image_count"`
	}

	galleryResponses := make([]GalleryResponse, len(galleries))
	for i := range galleries {
		// Get image count
		var count int64
		database.DB.Model(&models.Image{}).Where("gallery_id = ?", galleries[i].ID).Count(&count)

		// Load first image for thumbnail
		var firstImage models.Image
		if err := database.DB.Where("gallery_id = ?", galleries[i].ID).Order("created_at ASC").First(&firstImage).Error; err == nil {
			// Populate path
			sourceName := "uncategorized"
			if galleries[i].Source != nil {
				sourceName = galleries[i].Source.Name
			}
			sanitizedSource := services.SanitizeDirectoryName(sourceName)
			firstImage.WebPath = fmt.Sprintf("/images/%s", firstImage.Filename)
			firstImage.ThumbnailPath = strings.ReplaceAll(firstImage.Filename, sanitizedSource, fmt.Sprintf("/images/%s/thumbnails", sanitizedSource))

			galleries[i].Images = []models.Image{firstImage}
		}

		galleryResponses[i] = GalleryResponse{
			Gallery:    galleries[i],
			ImageCount: int(count),
		}
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))

	c.JSON(http.StatusOK, gin.H{
		"data": galleryResponses,
		"meta": gin.H{
			"current_page": page,
			"total_pages":  totalPages,
			"total_items":  total,
			"limit":        limit,
		},
	})
}

func GetGallery(c *gin.Context) {
	id := c.Param("id")
	var gallery models.Gallery
	if err := database.DB.Preload("Source").Preload("Images.Galleries").First(&gallery, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gallery not found"})
		return
	}

	// Populate paths
	sourceName := "uncategorized"
	if gallery.Source != nil {
		sourceName = gallery.Source.Name
	}
	sanitizedSource := services.SanitizeDirectoryName(sourceName)

	for i := range gallery.Images {
		gallery.Images[i].WebPath = fmt.Sprintf("/images/%s", gallery.Images[i].Filename)
		gallery.Images[i].ThumbnailPath = strings.ReplaceAll(gallery.Images[i].Filename, sanitizedSource, fmt.Sprintf("/images/%s/thumbnails", sanitizedSource))
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
	// Only load filenames if we're deleting images
	if deleteImages {
		if err := database.DB.Preload("Images", func(db *gorm.DB) *gorm.DB {
			return db.Select("id", "gallery_id", "filename")
		}).First(&gallery, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Gallery not found"})
			return
		}
	} else {
		if err := database.DB.First(&gallery, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Gallery not found"})
			return
		}
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
