package handlers

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
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

	// Try to auto-link to people based on source name
	linkedPersonIDs := autoLinkPeopleToGallery(gallery.Name, gallery.ID)

	// Check if this gallery matches any missing galleries by name (across all providers)
	if len(linkedPersonIDs) > 0 {
		if _, err := services.CheckAndLinkMissingGalleriesByName(gallery.ID, gallery.Name, linkedPersonIDs); err != nil {
			logger.Warnf("Failed to check for missing gallery matches by name: %v", err)
		}
	}

	// Check if this gallery matches any missing galleries from person scans (by URL)
	if gallery.Provider != "" && gallery.SourceURL != "" {
		if linkedIDs, err := services.CheckAndLinkFoundGallery(gallery.SourceURL, gallery.Name, gallery.Provider); err != nil {
			logger.Warnf("Failed to check for missing gallery matches: %v", err)
		} else if len(linkedIDs) > 0 {
			logger.Infof("Gallery %s auto-linked to %d people via missing gallery check", gallery.Name, len(linkedIDs))
		}
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

	if err := query.Limit(limit).Offset(offset).Find(&galleries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch galleries"})
		return
	}

	type GalleryResponse struct {
		models.Gallery
		ImageCount int `json:"image_count"`
	}

	galleryResponses := make([]GalleryResponse, len(galleries))
	if len(galleries) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"data": galleryResponses,
			"meta": gin.H{
				"current_page": page,
				"total_pages":  0,
				"total_items":  total,
				"limit":        limit,
			},
		})
		return
	}

	// Batch: collect all gallery IDs
	galleryIDs := make([]uint, len(galleries))
	for i, g := range galleries {
		galleryIDs[i] = g.ID
	}

	// Batch: image counts per gallery via image_galleries junction table
	type GalleryCount struct {
		GalleryID uint
		Count     int
	}
	var galleryCounts []GalleryCount
	database.DB.Table("image_galleries").
		Select("gallery_id, COUNT(*) as count").
		Where("gallery_id IN ?", galleryIDs).
		Group("gallery_id").
		Scan(&galleryCounts)

	countMap := make(map[uint]int, len(galleries))
	for _, gc := range galleryCounts {
		countMap[gc.GalleryID] = gc.Count
	}

	// Batch: first image per gallery (lowest image_id = first added)
	// Step 1: get the minimum image_id per gallery from the junction table (index-only, fast)
	type galleryFirstID struct {
		GalleryID uint
		ImageID   uint
	}
	var firstIDs []galleryFirstID
	database.DB.Table("image_galleries").
		Select("gallery_id, MIN(image_id) as image_id").
		Where("gallery_id IN ?", galleryIDs).
		Group("gallery_id").
		Scan(&firstIDs)

	// Step 2: collect unique image IDs and batch-fetch their details
	thumbImgIDs := make([]uint, 0, len(firstIDs))
	thumbGalleryMap := make(map[uint]uint, len(firstIDs)) // gallery_id → image_id
	for _, fi := range firstIDs {
		thumbImgIDs = append(thumbImgIDs, fi.ImageID)
		thumbGalleryMap[fi.GalleryID] = fi.ImageID
	}

	type galleryFirstImage struct {
		ID       uint
		Filename string
		Type     string
	}
	var thumbImgs []galleryFirstImage
	if len(thumbImgIDs) > 0 {
		database.DB.Model(&models.Image{}).
			Select("id, filename, type").
			Where("id IN ? AND deleted_at IS NULL", thumbImgIDs).
			Scan(&thumbImgs)
	}

	// Map image_id → image details
	thumbImgMap := make(map[uint]galleryFirstImage, len(thumbImgs))
	for _, img := range thumbImgs {
		thumbImgMap[img.ID] = img
	}

	// Build response
	// Build a map from gallery ID to the first non-deleted image
	for i := range galleries {
		g := &galleries[i]
		imageCount := countMap[g.ID]

		// If provider thumbnail is present, use it first
		if g.ProviderThumbnail != "" {
			filename := filepath.Base(g.ProviderThumbnail)
			g.Images = []models.Image{{
				ThumbnailPath: "/images/gallery_thumbnails/" + filename,
			}}
		} else if g.CoverImageID != nil {
			// If a manual cover image is set, attempt to use that image's filename from DB
			var img models.Image
			if err := database.DB.Select("id", "filename", "type").First(&img, *g.CoverImageID).Error; err == nil {
				sourceName := "uncategorized"
				if g.Source != nil {
					sourceName = g.Source.Name
				}
				sanitizedSource := services.SanitizeDirectoryName(sourceName)

				img.WebPath = fmt.Sprintf("/images/%s", filepath.ToSlash(img.Filename))
				thumbPath := img.Filename
				if img.Type == "video" {
					ext := filepath.Ext(thumbPath)
					thumbPath = strings.TrimSuffix(thumbPath, ext) + ".jpg"
				}
				parts := strings.Split(filepath.ToSlash(thumbPath), "/")
				if len(parts) > 1 {
					parts = append(parts[:len(parts)-1], append([]string{"thumbnails"}, parts[len(parts)-1:]...)...)
					img.ThumbnailPath = "/images/" + strings.Join(parts, "/")
				} else {
					img.ThumbnailPath = fmt.Sprintf("/images/%s/thumbnails/%s", sanitizedSource, filepath.Base(thumbPath))
				}
				g.Images = []models.Image{img}
			} else if imgID, hasThumb := thumbGalleryMap[g.ID]; hasThumb {
				if fi, ok := thumbImgMap[imgID]; ok {
					sourceName := "uncategorized"
					if g.Source != nil {
						sourceName = g.Source.Name
					}
					sanitizedSource := services.SanitizeDirectoryName(sourceName)

					firstImage := models.Image{
						ID:       fi.ID,
						Filename: fi.Filename,
						Type:     fi.Type,
					}
					firstImage.WebPath = fmt.Sprintf("/images/%s", filepath.ToSlash(firstImage.Filename))

					thumbPath := firstImage.Filename
					if firstImage.Type == "video" {
						ext := filepath.Ext(thumbPath)
						thumbPath = strings.TrimSuffix(thumbPath, ext) + ".jpg"
					}

					parts := strings.Split(filepath.ToSlash(thumbPath), "/")
					if len(parts) > 1 {
						parts = append(parts[:len(parts)-1], append([]string{"thumbnails"}, parts[len(parts)-1:]...)...)
						firstImage.ThumbnailPath = "/images/" + strings.Join(parts, "/")
					} else {
						firstImage.ThumbnailPath = fmt.Sprintf("/images/%s/thumbnails/%s", sanitizedSource, filepath.Base(thumbPath))
					}

					g.Images = []models.Image{firstImage}
				}
			}
		} else if imgID, hasThumb := thumbGalleryMap[g.ID]; hasThumb {
			if fi, ok := thumbImgMap[imgID]; ok {
				sourceName := "uncategorized"
				if g.Source != nil {
					sourceName = g.Source.Name
				}
				sanitizedSource := services.SanitizeDirectoryName(sourceName)

				firstImage := models.Image{
					ID:       fi.ID,
					Filename: fi.Filename,
					Type:     fi.Type,
				}
				firstImage.WebPath = fmt.Sprintf("/images/%s", filepath.ToSlash(firstImage.Filename))

				thumbPath := firstImage.Filename
				if firstImage.Type == "video" {
					ext := filepath.Ext(thumbPath)
					thumbPath = strings.TrimSuffix(thumbPath, ext) + ".jpg"
				}

				parts := strings.Split(filepath.ToSlash(thumbPath), "/")
				if len(parts) > 1 {
					parts = append(parts[:len(parts)-1], append([]string{"thumbnails"}, parts[len(parts)-1:]...)...)
					firstImage.ThumbnailPath = "/images/" + strings.Join(parts, "/")
				} else {
					firstImage.ThumbnailPath = fmt.Sprintf("/images/%s/thumbnails/%s", sanitizedSource, filepath.Base(thumbPath))
				}

				g.Images = []models.Image{firstImage}
			}
		}

		galleryResponses[i] = GalleryResponse{
			Gallery:    *g,
			ImageCount: imageCount,
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
	if err := database.DB.Preload("Source").Preload("People").First(&gallery, id).Error; err != nil {
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
		Name         string `json:"name"`
		CoverImageID *uint  `json:"cover_image_id"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	gallery.Name = input.Name
	if input.CoverImageID != nil {
		gallery.CoverImageID = input.CoverImageID
	}
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
		Provider     string `json:"provider" binding:"required"`
		SourceURL    string `json:"source_url"`
		CoverImageID *uint  `json:"cover_image_id"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	gallery.Provider = input.Provider
	if input.SourceURL != "" {
		gallery.SourceURL = input.SourceURL
	}
	if input.CoverImageID != nil {
		gallery.CoverImageID = input.CoverImageID
	}

	if err := database.DB.Save(&gallery).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update gallery provider"})
		return
	}

	c.JSON(http.StatusOK, gallery)
}
