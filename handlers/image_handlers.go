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
		DownloadURL: req.URL, // For manual additions, both are the same
	}
	if err := database.DB.Create(&image).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save image record"})
		return
	}

	c.JSON(http.StatusCreated, image)
}

// GetImages returns all images with pagination
func GetImages(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 100
	}
	offset := (page - 1) * limit

	var total int64
	database.DB.Model(&models.Image{}).Count(&total)

	var images []models.Image
	if err := database.DB.Preload("Gallery").Limit(limit).Offset(offset).Order("created_at DESC").Find(&images).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch images"})
		return
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))

	c.JSON(http.StatusOK, gin.H{
		"data": images,
		"meta": gin.H{
			"current_page": page,
			"total_pages":  totalPages,
			"total_items":  total,
			"limit":        limit,
		},
	})
}

func ServeImage(c *gin.Context) {
	filename := c.Param("filename")
	path := filepath.Join(services.UploadsDir, filename)
	c.File(path)
}

func ServeThumbnail(c *gin.Context) {
	filename := c.Param("filename")
	// Thumbnails are in the thumbnails subdirectory with the same filename
	path := filepath.Join(services.UploadsDir, "thumbnails", filename)
	c.File(path)
}

// DeleteImage removes an image from both database and filesystem
func DeleteImage(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid image ID"})
		return
	}

	// Get image details first
	var image models.Image
	if err := database.DB.First(&image, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
		return
	}

	// Delete from filesystem
	imagePath := filepath.Join(services.UploadsDir, image.Filename)
	if err := services.DeleteFile(imagePath); err != nil {
		// Log but don't fail if file doesn't exist
		println("Warning: Failed to delete image file:", err.Error())
	}

	// Delete thumbnail
	thumbnailPath := filepath.Join(services.UploadsDir, "thumbnails", image.Filename)
	if err := services.DeleteFile(thumbnailPath); err != nil {
		println("Warning: Failed to delete thumbnail:", err.Error())
	}

	// Delete from database
	if err := database.DB.Delete(&image).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete image"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Image deleted successfully"})
}
