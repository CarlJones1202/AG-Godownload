package handlers

import (
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
)

func CreateSource(c *gin.Context) {
	var source models.Source
	if err := c.ShouldBindJSON(&source); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check for duplicate location
	var existing models.Source
	if err := database.DB.Where("location = ?", source.Location).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"error":           "A source with this URL already exists",
			"existing_source": existing,
		})
		return
	}

	if err := database.DB.Create(&source).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create source"})
		return
	}

	// Automatically create a gallery for this source
	gallery := models.Gallery{
		Name:     source.Name,
		SourceID: &source.ID,
	}
	if err := database.DB.Create(&gallery).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create default gallery for source"})
		return
	}

	// Queue for crawling
	services.AddToCrawlerQueue(source.ID)

	c.JSON(http.StatusCreated, source)
}

func CrawlSource(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid source ID"})
		return
	}

	// Trigger crawl in background
	services.AddToCrawlerQueue(uint(id))

	c.JSON(http.StatusAccepted, gin.H{"message": "Crawl started"})
}

func GetSources(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit

	query := database.DB.Model(&models.Source{})

	// Search filter
	search := c.Query("q")
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("name LIKE ? OR location LIKE ?", searchPattern, searchPattern)
	}

	var total int64
	query.Count(&total)

	var sources []models.Source
	if err := query.Limit(limit).Offset(offset).Find(&sources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch sources"})
		return
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))

	c.JSON(http.StatusOK, gin.H{
		"data": sources,
		"meta": gin.H{
			"current_page": page,
			"total_pages":  totalPages,
			"total_items":  total,
			"limit":        limit,
		},
	})
}

// DeleteSource removes a source and optionally its gallery and images
func DeleteSource(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid source ID"})
		return
	}

	// Check cascade options
	deleteGallery := c.Query("delete_gallery") == "true"
	deleteImages := c.Query("delete_images") == "true"

	var source models.Source
	if err := database.DB.First(&source, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Source not found"})
		return
	}

	if deleteGallery || deleteImages {
		// Find associated gallery
		var gallery models.Gallery
		if err := database.DB.Preload("Images").Where("source_id = ?", id).First(&gallery).Error; err == nil {
			if deleteImages {
				// Delete all images
				sourceDir := services.SanitizeDirectoryName(source.Name)
				for _, image := range gallery.Images {
					// Filename might be just basename or relative path.
					// Construct path based on source name.
					imagePath := filepath.Join(services.UploadsDir, sourceDir, filepath.Base(image.Filename))

					// If file doesn't exist at constructed path, try utilizing the stored filename directly
					// in case it was stored as a relative path "source/file.jpg"
					if _, err := os.Stat(imagePath); os.IsNotExist(err) {
						directPath := filepath.Join(services.UploadsDir, image.Filename)
						if _, err := os.Stat(directPath); err == nil {
							imagePath = directPath
						}
					}

					services.DeleteFile(imagePath)

					// Handle thumbnail
					// Thumbnails are usually in uploads/source_name/thumbnails/filename
					thumbnailPath := filepath.Join(services.UploadsDir, sourceDir, "thumbnails", filepath.Base(image.Filename))
					services.DeleteFile(thumbnailPath)
				}
				database.DB.Where("gallery_id = ?", gallery.ID).Delete(&models.Image{})
			}
			if deleteGallery {
				database.DB.Delete(&gallery)
			}
		}
	}

	// Delete source
	if err := database.DB.Delete(&source).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete source"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Source deleted successfully"})
}
