package handlers

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"strings"

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
	result, err := services.DownloadImage(req.URL, sourceName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to download image: " + err.Error()})
		return
	}

	// Generate thumbnail
	_, err = services.GenerateThumbnail(result.Path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate thumbnail: " + err.Error()})
		return
	}

	// Save to DB
	// Get relative path from uploads directory (e.g., "SourceName/filename.jpg")
	relPath, err := filepath.Rel(services.UploadsDir, result.Path)
	if err != nil {
		// Fallback to just filename if Rel fails
		relPath = filepath.Join(sourceName, filepath.Base(result.Path))
	}

	image := models.Image{
		// GalleryID:   uint(galleryID), // Keep for backward compat if needed, but let's try to move to M2M
		Filename:       relPath,
		OriginalURL:    req.URL,
		DownloadURL:    req.URL,
		DominantColors: result.DominantColors,
		Galleries:      []*models.Gallery{&gallery},
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
	// Preload Galleries.Source to get source name
	if err := database.DB.Preload("Galleries.Source").Limit(limit).Offset(offset).Order("created_at DESC").Find(&images).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch images"})
		return
	}

	// Populate virtual paths
	for i := range images {
		sourceName := "uncategorized"
		if len(images[i].Galleries) > 0 && images[i].Galleries[0].SourceID != nil {
			// Check if Source is loaded
			if images[i].Galleries[0].Source.Name != "" {
				sourceName = images[i].Galleries[0].Source.Name
			} else {
				// Fallback if not loaded (should be loaded by Preload)
				var source models.Source
				if err := database.DB.First(&source, *images[i].Galleries[0].SourceID).Error; err == nil {
					sourceName = source.Name
				}
			}
		}

		sanitizedSource := services.SanitizeDirectoryName(sourceName)
		images[i].WebPath = fmt.Sprintf("/images/%s/%s", sanitizedSource, images[i].Filename)
		images[i].ThumbnailPath = fmt.Sprintf("/images/%s/thumbnails/%s", sanitizedSource, images[i].Filename)
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

// ServeImage - serves image from path provided in URL or falls back to DB lookup
func ServeImage(c *gin.Context) {
	filepathParam := c.Param("filepath")

	// Security: prevent path traversal (basic check, filepath.Clean handles more)
	if strings.Contains(filepathParam, "..") {
		c.String(http.StatusBadRequest, "Invalid filename")
		return
	}

	// Clean the path
	cleanPath := filepath.Clean(filepathParam)
	// Remove leading slash if present (filepath.Clean might leave it on unix, but we want relative to uploads)
	cleanPath = strings.TrimPrefix(cleanPath, "/")
	cleanPath = strings.TrimPrefix(cleanPath, "\\")

	// Construct full path
	fullPath := filepath.Join(services.UploadsDir, cleanPath)

	// Verify the path is still within UploadsDir (extra security)
	absUploads, _ := filepath.Abs(services.UploadsDir)
	absPath, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absPath, absUploads) {
		c.String(http.StatusForbidden, "Access denied")
		return
	}

	if fileExists(fullPath) {
		c.File(fullPath)
		return
	}

	// Fallback for backward compatibility or if path is just a filename
	// If the path doesn't contain separators, treat as filename and look up in DB
	if !strings.Contains(cleanPath, "/") && !strings.Contains(cleanPath, "\\") {
		filename := cleanPath
		var image struct {
			ID         uint
			Filename   string
			SourceName string `gorm:"column:source_name"`
		}

		err := database.DB.
			Model(&models.Image{}).
			Select(`images.id, images.filename, sources.name AS source_name`).
			Joins(`JOIN image_galleries ON image_galleries.image_id = images.id`).
			Joins(`JOIN galleries ON galleries.id = image_galleries.gallery_id`).
			Joins(`JOIN sources ON sources.id = galleries.source_id`).
			Where(`images.filename LIKE ? OR images.filename = ?`, "%/"+filename, filename).
			First(&image).Error

		if err == nil {
			sourceDir := services.SanitizeDirectoryName(image.SourceName)
			path := filepath.Join(services.UploadsDir, sourceDir, filename)
			if fileExists(path) {
				c.File(path)
				return
			}
		}

		// Try root
		rootPath := filepath.Join(services.UploadsDir, filename)
		if fileExists(rootPath) {
			c.File(rootPath)
			return
		}
	}

	c.String(http.StatusNotFound, "Image not found")
}

// ServeThumbnail - Deprecated, handled by ServeImage with nested path
func ServeThumbnail(c *gin.Context) {
	ServeImage(c)
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
