package handlers

import (
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func CreateSource(c *gin.Context) {
	var source models.Source
	if err := c.ShouldBindJSON(&source); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := createSingleSource(source.Name, source.Location, source.Type, source.Priority)
	if err != nil {
		c.JSON(err.Code, gin.H{"error": err.Message})
		return
	}

	c.JSON(http.StatusCreated, created)
}

type importError struct {
	Code    int
	Message string
}

func createSingleSource(name, location, sourceType string, priority int) (*models.Source, *importError) {
	source := models.Source{
		Name:     name,
		Type:     sourceType,
		Location: location,
		Priority: priority,
		Status:   "idle",
	}

	// Default type if not set
	if source.Type == "" {
		source.Type = "url"
	}

	// Check for duplicate location
	var existing models.Source
	if err := database.DB.Where("location = ?", source.Location).First(&existing).Error; err == nil {
		return nil, &importError{http.StatusConflict, "A source with this URL already exists"}
	}

	if err := database.DB.Create(&source).Error; err != nil {
		return nil, &importError{http.StatusInternalServerError, "Failed to create source"}
	}

	// Only create a gallery for non-video sources (videos are stored standalone)
	isVideo := services.IsVideoURL(source.Location) || services.IsVideoFile(source.Location)
	if !isVideo {
		// Automatically create a gallery for this source
		gallery := models.Gallery{
			Name:     source.Name,
			SourceID: &source.ID,
		}
		if err := database.DB.Create(&gallery).Error; err != nil {
			return nil, &importError{http.StatusInternalServerError, "Failed to create default gallery for source"}
		}

		// Try to auto-link to people based on source name
		linkedPersonIDs := autoLinkPeopleToGallery(source.Name, gallery.ID)

		// Check if this gallery matches any missing galleries by name (across all providers)
		if len(linkedPersonIDs) > 0 {
			if _, err := services.CheckAndLinkMissingGalleriesByName(gallery.ID, source.Name, linkedPersonIDs); err != nil {
				logger.Warnf("Failed to check for missing gallery matches: %v", err)
			}
		}
	}

    // Queue for crawling (use video queue for known video sources)
    if isVideo {
        services.AddToVideoQueue(source.ID)
    } else {
        services.AddToCrawlerQueue(source.ID)
    }

	return &source, nil
}

func BulkImportSources(c *gin.Context) {
	var inputs []struct {
		URL  string  `json:"url"`
		Name *string `json:"name"`
	}
	if err := c.ShouldBindJSON(&inputs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	type resultEntry struct {
		URL     string `json:"url"`
		Name    string `json:"name"`
		Status  string `json:"status"`
		SourceID *uint  `json:"source_id,omitempty"`
		Error  string `json:"error,omitempty"`
	}

	results := make([]resultEntry, 0, len(inputs))
	created := 0
	duplicates := 0
	failed := 0

	for _, input := range inputs {
		entry := resultEntry{
			URL:  input.URL,
			Name: "",
		}

		name := ""
		if input.Name != nil {
			name = *input.Name
			entry.Name = name
		}

		createdSource, err := createSingleSource(name, input.URL, "url", 0)
		if err != nil {
			entry.Status = "failed"
			if err.Code == http.StatusConflict {
				entry.Status = "duplicate"
				duplicates++
			} else {
				failed++
			}
			entry.Error = err.Message

			// Look up the existing source for duplicate entries
			if err.Code == http.StatusConflict {
				var existing models.Source
				if err := database.DB.Where("location = ?", input.URL).First(&existing).Error; err == nil {
					entry.SourceID = &existing.ID
				}
			}
		} else {
			entry.Status = "created"
			entry.SourceID = &createdSource.ID
			created++
		}

		results = append(results, entry)
	}

	c.JSON(http.StatusOK, gin.H{
		"results":    results,
		"summary": gin.H{
			"total":      len(inputs),
			"created":    created,
			"duplicates": duplicates,
			"failed":     failed,
		},
	})
}

func CrawlSource(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid source ID"})
		return
	}

    // Trigger crawl in background - route to video queue if applicable
    var src models.Source
    if err := database.DB.Select("location").First(&src, id).Error; err == nil {
        if services.IsVideoURL(src.Location) || services.IsVideoFile(src.Location) {
            services.AddToVideoQueue(uint(id))
        } else {
            services.AddToCrawlerQueue(uint(id))
        }
    } else {
        services.AddToCrawlerQueue(uint(id))
    }

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

func UpdateSourcePriority(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid source ID"})
		return
	}

	var input struct {
		Priority int `json:"priority"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := database.DB.Model(&models.Source{}).Where("id = ?", id).Update("priority", input.Priority).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update priority"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Priority updated", "priority": input.Priority})
}

func GetDownloadStatus(c *gin.Context) {
	status := services.GetGlobalDownloadStatus()

	// Add currently crawling sources from DB for complete picture
	var activeSources []models.Source
	database.DB.Where("status = ?", "crawling").Find(&activeSources)

	// We could also refine the status struct to include source details
	// but for now, the UI can match by ID from the sources list.

	c.JSON(http.StatusOK, status)
}

// autoLinkPeopleToGallery attempts to link people to a gallery based on name matching
// Returns the list of person IDs that were linked
func autoLinkPeopleToGallery(sourceName string, galleryID uint) []uint {
	if sourceName == "" {
		return nil
	}

	// Search for people with matching names (case-insensitive partial match)
	searchPattern := "%" + sourceName + "%"
	var people []models.Person
	database.DB.Where("LOWER(name) LIKE ?", strings.ToLower(searchPattern)).Find(&people)

	if len(people) == 0 {
		return nil
	}

	// Get the gallery
	var gallery models.Gallery
	if err := database.DB.First(&gallery, galleryID).Error; err != nil {
		return nil
	}

	// Find matching people and link them
	var matchedGalleries []*models.Gallery
	matchedGalleries = append(matchedGalleries, &gallery)

	var linkedPersonIDs []uint

	for i := range people {
		person := &people[i]
		// Check if name matches (case-insensitive)
		personNameLower := strings.ToLower(person.Name)
		sourceNameLower := strings.ToLower(sourceName)

		if strings.Contains(personNameLower, sourceNameLower) || strings.Contains(sourceNameLower, personNameLower) {
			// Use GORM's Association to append to existing galleries
			database.DB.Model(person).Association("Galleries").Append(&gallery)
			linkedPersonIDs = append(linkedPersonIDs, person.ID)
		}
	}

	return linkedPersonIDs
}
