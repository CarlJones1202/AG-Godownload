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

	// Base query
	db := database.DB.Model(&models.Gallery{})

	// Apply search if provided
	searchQuery := c.Query("q")
	if searchQuery != "" {
		searchTerm := "%" + strings.ToLower(searchQuery) + "%"
		db = db.Where("LOWER(name) LIKE ? OR LOWER(description) LIKE ?", searchTerm, searchTerm)
	}

	var total int64
	db.Count(&total)

	sortBy := c.DefaultQuery("sort", "newest")
	seedStr := c.Query("seed")
	seed, _ := strconv.Atoi(seedStr)

	var galleries []models.Gallery
	query := db.Preload("Source")

	switch sortBy {
	case "newest":
		query = query.Order("created_at DESC")
	case "oldest":
		query = query.Order("created_at ASC")
	case "shuffle":
		query = query.Order(fmt.Sprintf("(((id + 1) * 1103515245 + %d * 12345) %% 2147483647)", seed))
	default:
		query = query.Order("id DESC")
	}

	// Load galleries with Source preloaded
	if err := query.Limit(limit).Offset(offset).Find(&galleries).Error; err != nil {
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
		var images []models.Image
		if err := database.DB.Where("gallery_id = ?", galleries[i].ID).Order("created_at ASC").Limit(1).Find(&images).Error; err == nil && len(images) > 0 {
			firstImage := images[0]
			// Populate path
			sourceName := "uncategorized"
			if galleries[i].Source != nil {
				sourceName = galleries[i].Source.Name
			}
			sanitizedSource := services.SanitizeDirectoryName(sourceName)
			firstImage.WebPath = fmt.Sprintf("/images/%s", filepath.ToSlash(firstImage.Filename))

			// Construct thumbnail path
			thumbPath := firstImage.Filename
			if firstImage.Type == "video" {
				// Replace extension with .jpg for video thumbnails
				ext := filepath.Ext(thumbPath)
				thumbPath = strings.TrimSuffix(thumbPath, ext) + ".jpg"
			}

			// Inject "thumbnails" into the path
			// Assumption: Filename is like "Source/file.ext"
			// We want "/images/Source/thumbnails/file.ext"
			parts := strings.Split(filepath.ToSlash(thumbPath), "/")
			if len(parts) > 1 {
				// Insert "thumbnails" before the filename
				parts = append(parts[:len(parts)-1], append([]string{"thumbnails"}, parts[len(parts)-1:]...)...)
				firstImage.ThumbnailPath = "/images/" + strings.Join(parts, "/")
			} else {
				// Fallback
				firstImage.ThumbnailPath = fmt.Sprintf("/images/%s/thumbnails/%s", sanitizedSource, filepath.Base(thumbPath))
			}

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
	sortBy := c.DefaultQuery("sort", "newest")
	seedStr := c.Query("seed")
	seed, _ := strconv.Atoi(seedStr)

	var gallery models.Gallery
	query := database.DB.Preload("Source").Preload("People")

	// Preload images with the requested sort order
	query = query.Preload("Images", func(db *gorm.DB) *gorm.DB {
		subQuery := db.Preload("Galleries.Source")
		switch sortBy {
		case "newest":
			return subQuery.Order("created_at DESC")
		case "oldest":
			return subQuery.Order("created_at ASC")
		case "shuffle":
			return subQuery.Order(fmt.Sprintf("(((id + 1) * 1103515245 + %d * 12345) %% 2147483647)", seed))
		default:
			return subQuery.Order("created_at DESC")
		}
	})

	if err := query.First(&gallery, id).Error; err != nil {
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
		gallery.Images[i].WebPath = fmt.Sprintf("/images/%s", filepath.ToSlash(gallery.Images[i].Filename))

		thumbPath := gallery.Images[i].Filename
		if gallery.Images[i].Type == "video" {
			ext := filepath.Ext(thumbPath)
			thumbPath = strings.TrimSuffix(thumbPath, ext) + ".jpg"
		}

		parts := strings.Split(filepath.ToSlash(thumbPath), "/")
		if len(parts) > 1 {
			parts = append(parts[:len(parts)-1], append([]string{"thumbnails"}, parts[len(parts)-1:]...)...)
			gallery.Images[i].ThumbnailPath = "/images/" + strings.Join(parts, "/")
		} else {
			gallery.Images[i].ThumbnailPath = fmt.Sprintf("/images/%s/thumbnails/%s", sanitizedSource, filepath.Base(thumbPath))
		}
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

// UpdateGallery updates a gallery's details
func UpdateGallery(c *gin.Context) {
	id := c.Param("id")
	var gallery models.Gallery
	if err := database.DB.First(&gallery, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gallery not found"})
		return
	}

	var input struct {
		Name string `json:"name"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	gallery.Name = input.Name
	if err := database.DB.Save(&gallery).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update gallery"})
		return
	}

	c.JSON(http.StatusOK, gallery)
}

func UpdateGalleryProvider(c *gin.Context) {
	id := c.Param("id")
	var gallery models.Gallery
	if err := database.DB.First(&gallery, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gallery not found"})
		return
	}

	var input struct {
		Provider  string `json:"provider" binding:"required"`
		SourceURL string `json:"source_url"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	gallery.Provider = input.Provider
	if input.SourceURL != "" {
		gallery.SourceURL = input.SourceURL
	}

	if err := database.DB.Save(&gallery).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update gallery provider"})
		return
	}

	c.JSON(http.StatusOK, gallery)
}
