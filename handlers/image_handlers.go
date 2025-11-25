package handlers

import (
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
)

// AddImageToGallery handles downloading an image from a URL and adding it to a gallery
func AddImageToGallery(c *gin.Context) {
	galleryIDStr := c.Param("id")
	galleryID, err := strconv.Atoi(galleryIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid gallery ID"})
		return
	}

	var req struct {
		URL      string `json:"url"`
		Filename string `json:"filename"` // Optional
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify gallery exists
	var gallery models.Gallery
	if err := database.DB.First(&gallery, galleryID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gallery not found"})
		return
	}

	// Use filename from URL if not provided
	if req.Filename == "" {
		req.Filename = filepath.Base(req.URL)
	}

	// Download image
	destPath, err := services.DownloadImage(req.URL, req.Filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to download image: " + err.Error()})
		return
	}

	// Generate thumbnail
	_, err = services.GenerateThumbnail(destPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate thumbnail: " + err.Error()})
		return
	}

	// Save to DB
	image := models.Image{
		GalleryID:   uint(galleryID),
		Filename:    filepath.Base(destPath),
		OriginalURL: req.URL,
	}
	if err := database.DB.Create(&image).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save image record"})
		return
	}

	c.JSON(http.StatusCreated, image)
}

func ServeImage(c *gin.Context) {
	filename := c.Param("filename")
	path := filepath.Join(services.UploadsDir, filename)
	c.File(path)
}

func ServeThumbnail(c *gin.Context) {
	filename := c.Param("filename")
	// Assuming thumbnail naming convention from service
	ext := filepath.Ext(filename)
	name := filename[:len(filename)-len(ext)]
	thumbName := name + "_thumb.jpg"
	path := filepath.Join(services.UploadsDir, thumbName)
	c.File(path)
}
