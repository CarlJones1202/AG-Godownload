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
	// Construct relative path: SourceName/Filename
	sanitizedSource := services.SanitizeDirectoryName(sourceName)
	if sanitizedSource == "" {
		sanitizedSource = "unknown"
	}
	// relPath := filepath.Join(sanitizedSource, filepath.Base(result.Path))
	// We want to store just the filename (basename) to be consistent with crawler logic
	// The source folder is implicit via the Source relationship.
	filename := filepath.Base(result.Path)

	image := models.Image{
		// GalleryID:   uint(galleryID), // Keep for backward compat if needed, but let's try to move to M2M
		Filename:       filename,
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

// GetImages returns all images with pagination, filtering, and sorting
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
	query := database.DB.Model(&models.Image{})

	// Type filter
	filterType := c.Query("type")
	if filterType == "" {
		filterType = "image" // Default to showing only images to preserve backward compat
	}
	if filterType != "all" {
		query = query.Where("type = ?", filterType)
	}

	// Tag filter
	if tagIDs := c.Query("filter_tags"); tagIDs != "" {
		query = query.Joins("JOIN image_tags ON image_tags.image_id = images.id").
			Where("image_tags.tag_id IN (?)", strings.Split(tagIDs, ",")).
			Distinct()
	}

	// Person filter
	if personID := c.Query("filter_person"); personID != "" {
		query = query.Joins("JOIN person_images ON person_images.image_id = images.id").
			Where("person_images.person_id = ?", personID).
			Distinct()
	}

	// Color filter (approximate match)
	if color := c.Query("filter_color"); color != "" {
		query = query.Where("dominant_colors LIKE ?", "%"+color+"%")
	}

	// Sorting
	sortBy := c.DefaultQuery("sort", "newest")
	switch sortBy {
	case "newest":
		query = query.Order("created_at DESC")
	case "oldest":
		query = query.Order("created_at ASC")
	case "largest":
		query = query.Order("size_mb DESC")
	case "smallest":
		query = query.Order("size_mb ASC")
	case "random":
		query = query.Order("RANDOM()")
	default:
		query = query.Order("created_at DESC")
	}

	query.Count(&total)

	var images []models.Image
	// Preload Galleries.Source to get source name for images
	// Preload Source to get source name for videos (direct association)
	// Preload People for videos (direct association)
	// Preload Tags for all images
	if err := query.Preload("Galleries.Source").Preload("Source").Preload("People").Preload("Tags").Limit(limit).Offset(offset).Find(&images).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch images"})
		return
	}

	// Populate virtual paths
	for i := range images {
		sourceName := "uncategorized"

		// For videos, check direct Source association first
		if images[i].Type == "video" && images[i].SourceID != nil {
			if images[i].Source != nil && images[i].Source.Name != "" {
				sourceName = images[i].Source.Name
			} else {
				// Fallback: load source if not preloaded
				var source models.Source
				if err := database.DB.First(&source, *images[i].SourceID).Error; err == nil {
					sourceName = source.Name
				}
			}
		} else if len(images[i].Galleries) > 0 && images[i].Galleries[0].SourceID != nil {
			// For images, use gallery source
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
		images[i].WebPath = fmt.Sprintf("/images/%s/%s", sanitizedSource, filepath.Base(images[i].Filename))

		// Video thumbnails have .jpg appended to the filename
		thumbnailFilename := images[i].Filename
		if images[i].Type == "video" {
			thumbnailFilename = images[i].Filename + ".jpg"

			// Add trickplay paths for videos
			baseFilename := strings.TrimSuffix(filepath.Base(images[i].Filename), filepath.Ext(images[i].Filename))
			images[i].TrickplayVTT = fmt.Sprintf("/images/%s/trickplay/%s.vtt", sanitizedSource, baseFilename)
			images[i].TrickplaySprite = fmt.Sprintf("/images/%s/trickplay/%s_sprite.jpg", sanitizedSource, baseFilename)
		}
		images[i].ThumbnailPath = fmt.Sprintf("/images/%s/thumbnails/%s", sanitizedSource, filepath.Base(thumbnailFilename))
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
	// Get image details first (preload galleries and source)
	var image models.Image
	if err := database.DB.Preload("Galleries.Source").First(&image, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
		return
	}

	// Determine source directory
	sourceDir := "uncategorized"
	if len(image.Galleries) > 0 {
		if image.Galleries[0].Source != nil {
			sourceDir = services.SanitizeDirectoryName(image.Galleries[0].Source.Name)
		} else if image.Galleries[0].SourceID != nil {
			var source models.Source
			if err := database.DB.First(&source, *image.Galleries[0].SourceID).Error; err == nil {
				sourceDir = services.SanitizeDirectoryName(source.Name)
			}
		}
	}

	// Delete from filesystem
	// Try constructed path first
	imagePath := filepath.Join(services.UploadsDir, sourceDir, filepath.Base(image.Filename))

	// Fallback to direct check if not found (in case it was stored as relative path)
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		directPath := filepath.Join(services.UploadsDir, image.Filename)
		if _, err := os.Stat(directPath); err == nil {
			imagePath = directPath
			// If we found it via direct path which includes directory, we might need to adjust logic
			// But usually directPath is for legacy cases.
			// Re-eval sourceDir for thumbnail? If image.Filename was "source/file.jpg", Base is "file.jpg".
		}
	}

	if err := services.DeleteFile(imagePath); err != nil {
		// Log but don't fail if file doesn't exist
		println("Warning: Failed to delete image file:", err.Error())
	}

	// Delete thumbnail
	// Delete thumbnail
	thumbnailPath := filepath.Join(services.UploadsDir, sourceDir, "thumbnails", filepath.Base(image.Filename))
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
