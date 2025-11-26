package handlers

import (
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"strings"

	"github.com/gin-gonic/gin"

	"gorm.io/gorm"
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

	// Determine source name
	var sourceName string
	if gallery.SourceID != nil {
		var source models.Source
		if err := database.DB.First(&source, *gallery.SourceID).Error; err == nil {
			sourceName = source.Name
		} else {
			sourceName = "uncategorized"
		}
	} else {
		sourceName = "uncategorized"
	}

	// Download image
	destPath, err := services.DownloadImage(req.URL, sourceName)
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
		// GalleryID:   uint(galleryID), // Keep for backward compat if needed, but let's try to move to M2M
		Filename:    filepath.Base(destPath),
		OriginalURL: req.URL,
		DownloadURL: req.URL,
		Galleries:   []*models.Gallery{&gallery},
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
	if err := database.DB.Preload("Galleries").Limit(limit).Offset(offset).Order("created_at DESC").Find(&images).Error; err != nil {
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

// ServeImage - finds image by filename, determines source name via DB, serves from correct subdir
func ServeImage(c *gin.Context) {
	filename := c.Param("filename")

	// Security: prevent path traversal
	if strings.Contains(filename, "/") || strings.Contains(filename, "..") {
		c.String(http.StatusBadRequest, "Invalid filename")
		return
	}

	var image struct {
		ID         uint
		Filename   string
		SourceName string `gorm:"column:source_name"`
	}

	// Find image by filename (hash), join through galleries → source
	err := database.DB.
		Model(&models.Image{}).
		Select(`images.id, images.filename, sources.name AS source_name`).
		Joins(`JOIN image_galleries ON image_galleries.image_id = images.id`).
		Joins(`JOIN galleries ON galleries.id = image_galleries.gallery_id`).
		Joins(`JOIN sources ON sources.id = galleries.source_id`).
		Where(`images.filename LIKE ? OR images.filename = ?`, "%/"+filename, filename).
		First(&image).Error

	// Fallback: if no source (manual/uncategorized), try root
	if err == gorm.ErrRecordNotFound {
		path := filepath.Join(services.UploadsDir, filename)
		if fileExists(path) {
			c.File(path)
			return
		}
		c.String(http.StatusNotFound, "Image not found")
		return
	}

	if err != nil {
		c.String(http.StatusInternalServerError, "Database error")
		return
	}

	// Build path using Source.Name as subdirectory
	sourceDir := services.SanitizeDirectoryName(image.SourceName)
	path := filepath.Join(services.UploadsDir, sourceDir, filename)

	if !fileExists(path) {
		// Last-ditch: try root (in case of migration issues)
		rootPath := filepath.Join(services.UploadsDir, filename)
		if fileExists(rootPath) {
			c.File(rootPath)
			return
		}
		c.String(http.StatusNotFound, "Image file missing")
		return
	}

	c.File(path)
}

// ServeThumbnail - same logic, but for thumbnails
func ServeThumbnail(c *gin.Context) {
	filename := c.Param("filename")

	if strings.Contains(filename, "/") || strings.Contains(filename, "..") {
		c.String(http.StatusBadRequest, "Invalid filename")
		return
	}

	var image struct {
		SourceName string `gorm:"column:source_name"`
	}

	err := database.DB.
		Model(&models.Image{}).
		Select(`sources.name AS source_name`).
		Joins(`JOIN image_galleries ON image_galleries.image_id = images.id`).
		Joins(`JOIN galleries ON galleries.id = image_galleries.gallery_id`).
		Joins(`JOIN sources ON sources.id = galleries.source_id`).
		Where(`images.filename LIKE ? OR images.filename = ?`, "%/"+filename, filename).
		First(&image).Error

	if err == gorm.ErrRecordNotFound {
		// Try old flat thumbnail
		oldPath := filepath.Join(services.UploadsDir, "thumbnails", filename)
		if fileExists(oldPath) {
			c.File(oldPath)
			return
		}
		c.String(http.StatusNotFound, "Thumbnail not found")
		return
	}

	if err != nil {
		c.String(http.StatusInternalServerError, "Database error")
		return
	}

	sourceDir := services.SanitizeDirectoryName(image.SourceName)
	thumbPath := filepath.Join(services.UploadsDir, sourceDir, "thumbnails", filename)

	if fileExists(thumbPath) {
		c.File(thumbPath)
		return
	}

	// Fallback to root thumbnails
	oldPath := filepath.Join(services.UploadsDir, "thumbnails", filename)
	if fileExists(oldPath) {
		c.File(oldPath)
		return
	}

	c.String(http.StatusNotFound, "Thumbnail missing")
}

// Helper
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
