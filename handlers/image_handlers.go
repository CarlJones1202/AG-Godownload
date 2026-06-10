package handlers

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	urlpkg "net/url"
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
	// Use the request URL origin as referer to improve success on hosts that validate referer
	referer := ""
	if u, errp := urlpkg.Parse(req.URL); errp == nil {
		referer = u.Scheme + "://" + u.Host
	}
	result, err := services.DownloadImage(req.URL, sourceName, referer)
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

	// Support offset-based pagination (used by the new UI) as well as page-based
	offset := (page - 1) * limit
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil {
			offset = parsedOffset
			// Recalculate page from offset for the response meta
			page = (offset / limit) + 1
		}
	}

	var total int64
	query := database.DB.Model(&models.Image{})

	// Type filter — accept both "type" and "is_video" from the frontend
	filterType := c.Query("type")
	if filterType == "" {
		if c.Query("is_video") == "true" {
			filterType = "video"
		} else if c.Query("is_video") == "false" {
			filterType = "image"
		} else {
			filterType = "image"
		}
	}
	if filterType != "all" {
		query = query.Where("type = ?", filterType)
	}

	// Use subqueries instead of JOIN+DISTINCT for M2M filter conditions.
	// This lets SQLite use covering indexes on both the junction table and images,
	// avoiding a wide DISTINCT scan of all columns.
	if galleryIDStr := c.Query("gallery_id"); galleryIDStr != "" {
		if galleryID, err := strconv.Atoi(galleryIDStr); err == nil {
			query = query.Where("id IN (SELECT image_id FROM image_galleries WHERE gallery_id = ?)", galleryID)
		}
	}
	if tagIDs := c.Query("filter_tags"); tagIDs != "" {
		query = query.Where("id IN (SELECT image_id FROM image_tags WHERE tag_id IN (?))", strings.Split(tagIDs, ","))
	}
	if personID := c.Query("filter_person"); personID != "" {
		query = query.Where("id IN (SELECT image_id FROM person_images WHERE person_id = ?)", personID)
	}

	// Color filter (approximate match)
	if color := c.Query("filter_color"); color != "" {
		query = query.Where("dominant_colors LIKE ?", "%"+color+"%")
	}

	// Favorites filter — accept both "favorites" and "is_favorite"
	if c.Query("favorites") == "true" || c.Query("is_favorite") == "true" {
		query = query.Where("is_favorite = ?", true)
	}

	// Exists on disk filter — accept both "exists" and "on_disk"
	existsFilter := c.Query("exists")
	if existsFilter == "" {
		existsFilter = c.Query("on_disk")
	}
	applyExistsFilter := existsFilter != ""

	// Count total matching images using a separate query chain
	countQuery := database.DB.Model(&models.Image{})
	if filterType != "all" {
		countQuery = countQuery.Where("type = ?", filterType)
	}
	if galleryIDStr := c.Query("gallery_id"); galleryIDStr != "" {
		if galleryID, err := strconv.Atoi(galleryIDStr); err == nil {
			countQuery = countQuery.Where("id IN (SELECT image_id FROM image_galleries WHERE gallery_id = ?)", galleryID)
		}
	}
	if tagIDs := c.Query("filter_tags"); tagIDs != "" {
		countQuery = countQuery.Where("id IN (SELECT image_id FROM image_tags WHERE tag_id IN (?))", strings.Split(tagIDs, ","))
	}
	if personID := c.Query("filter_person"); personID != "" {
		countQuery = countQuery.Where("id IN (SELECT image_id FROM person_images WHERE person_id = ?)", personID)
	}
	if color := c.Query("filter_color"); color != "" {
		countQuery = countQuery.Where("dominant_colors LIKE ?", "%"+color+"%")
	}
	if c.Query("favorites") == "true" || c.Query("is_favorite") == "true" {
		countQuery = countQuery.Where("is_favorite = ?", true)
	}
	countQuery.Count(&total)

	// Sorting — accept both "sort" and "sort_by"
	sortBy := c.DefaultQuery("sort", "")
	if sortBy == "" {
		sortBy = c.DefaultQuery("sort_by", "newest")
	}
	seedStr := c.Query("seed")
	if seedStr == "" {
		seedStr = c.Query("random_seed")
	}
	seed, _ := strconv.Atoi(seedStr)

	switch sortBy {
	case "newest":
		query = query.Order("created_at DESC, id DESC")
	case "oldest":
		query = query.Order("created_at ASC, id ASC")
	case "largest":
		query = query.Order("size_mb DESC, id DESC")
	case "smallest":
		query = query.Order("size_mb ASC, id ASC")
	case "random":
		query = query.Order("RANDOM()")
	case "shuffle":
		// Deterministic seeded shuffle using a simple LCG-style formula
		// ((id * multiplier + seed) % modulus)
		// We use a large prime multiplier and 2^31-1 as modulus
		query = query.Order(fmt.Sprintf("(((id + 1) * 1103515245 + %d * 12345) %% 2147483647)", seed))
	default:
		query = query.Order("created_at DESC, id DESC")
	}

	// Fetch images with pagination
	// If exists filter is applied, we'll fetch extra and filter client-side
	fetchLimit := limit
	if applyExistsFilter {
		// Fetch more to account for filtered out images
		fetchLimit = limit * 3
	}

	var images []models.Image
	// Preload Galleries.Source to get source name for images
	// Preload Source to get source name for videos (direct association)
	// Preload People for videos (direct association)
	// Preload Tags for all images
	if err := query.Preload("Galleries.Source").Preload("Source").Preload("People").Preload("Tags").Limit(fetchLimit).Offset(offset).Find(&images).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch images"})
		return
	}

	// Apply exists filter if enabled - check filesystem only for fetched images
	if applyExistsFilter {
		filteredImages := make([]models.Image, 0, len(images))
		for _, img := range images {
			fullPath := filepath.Join(services.UploadsDir, img.Filename)
			if _, err := os.Stat(fullPath); err == nil {
				if existsFilter == "true" {
					filteredImages = append(filteredImages, img)
				}
			} else {
				if existsFilter == "false" {
					filteredImages = append(filteredImages, img)
				}
			}
			if len(filteredImages) >= limit {
				break
			}
		}
		images = filteredImages
	}

	// Populate virtual paths
	for i := range images {
		sourceName := "uncategorized"

		if images[i].Type == "video" && images[i].SourceID != nil && images[i].Source != nil {
			sourceName = images[i].Source.Name
		} else if len(images[i].Galleries) > 0 && images[i].Galleries[0].Source != nil {
			sourceName = images[i].Galleries[0].Source.Name
		}

		sanitizedSource := services.SanitizeDirectoryName(sourceName)
		images[i].WebPath = fmt.Sprintf("/images/%s/%s", sanitizedSource, filepath.Base(images[i].Filename))

		// Video thumbnails use base filename (without video extension) + .jpg
		thumbnailFilename := images[i].Filename
		if images[i].Type == "video" {
			baseFilename := strings.TrimSuffix(images[i].Filename, filepath.Ext(images[i].Filename))
			thumbnailFilename = baseFilename + ".jpg"

			baseFilename = strings.TrimSuffix(filepath.Base(images[i].Filename), filepath.Ext(images[i].Filename))

			// Add trickplay paths for videos
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

// ServeThumbnail serves thumbnail images from the uploads/<source>/thumbnails/ directory
func ServeThumbnail(c *gin.Context) {
	filepathParam := c.Param("filepath")

	// Security: prevent path traversal
	if strings.Contains(filepathParam, "..") {
		c.String(http.StatusBadRequest, "Invalid filename")
		return
	}

	// Clean the path
	cleanPath := filepath.Clean(filepathParam)
	cleanPath = strings.TrimPrefix(cleanPath, "/")
	cleanPath = strings.TrimPrefix(cleanPath, "\\")

	// Always normalise to .jpg extension since thumbnails are generated as JPEG
	ext := filepath.Ext(cleanPath)
	if ext != ".jpg" && ext != ".jpeg" {
		cleanPath = strings.TrimSuffix(cleanPath, ext) + ".jpg"
	}

	// Verify the resolved path stays within UploadsDir
	absUploads, _ := filepath.Abs(services.UploadsDir)

	// Case 1: Path already contains a source directory (e.g., "SourceName/hash.jpg")
	// We inject "thumbnails" before the filename: uploads/SourceName/thumbnails/hash.jpg
	parts := strings.Split(filepath.ToSlash(cleanPath), "/")
	if len(parts) > 1 {
		sourceDir := strings.Join(parts[:len(parts)-1], "/")
		filename := parts[len(parts)-1]
		thumbPath := filepath.Join(services.UploadsDir, sourceDir, "thumbnails", filename)
		absPath, _ := filepath.Abs(thumbPath)
		if strings.HasPrefix(absPath, absUploads) && fileExists(thumbPath) {
			c.File(thumbPath)
			return
		}
	}

	// Case 2: Plain filename — look up in DB to find the source name
	filename := filepath.Base(cleanPath)
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
		Where(`images.filename LIKE ? OR images.filename = ?`, "%"+filename[:len(filename)-4]+"%", strings.TrimSuffix(filename, ".jpg")).
		First(&image).Error

	if err == nil {
		sourceDir := services.SanitizeDirectoryName(image.SourceName)
		thumbPath := filepath.Join(services.UploadsDir, sourceDir, "thumbnails", filename)
		absPath, _ := filepath.Abs(thumbPath)
		if strings.HasPrefix(absPath, absUploads) && fileExists(thumbPath) {
			c.File(thumbPath)
			return
		}
	}

	// Case 3: Check gallery_thumbnails directory (for provider thumbnails)
	galleryThumbPath := filepath.Join(services.UploadsDir, "gallery_thumbnails", filename)
	absGalleryPath, _ := filepath.Abs(galleryThumbPath)
	if strings.HasPrefix(absGalleryPath, absUploads) && fileExists(galleryThumbPath) {
		c.File(galleryThumbPath)
		return
	}

	// Case 4: Brute-force search across all source directories for the thumbnail
	entries, _ := os.ReadDir(services.UploadsDir)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(services.UploadsDir, entry.Name(), "thumbnails", filename)
		absCandidate, _ := filepath.Abs(candidate)
		if strings.HasPrefix(absCandidate, absUploads) && fileExists(candidate) {
			c.File(candidate)
			return
		}
	}

	c.String(http.StatusNotFound, "Thumbnail not found")
}

// Helper
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DeleteImage removes an image or video from both database and filesystem
func DeleteImage(c *gin.Context) {
	idStr := c.Param("imageId")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid image ID"})
		return
	}

	// Get image details first (preload galleries, source for videos)
	var image models.Image
	if err := database.DB.Preload("Galleries.Source").Preload("Source").First(&image, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
		return
	}

	// Determine source directory
	sourceDir := "uncategorized"

	// For videos, check direct Source association first
	if image.Type == "video" && image.SourceID != nil {
		if image.Source != nil && image.Source.Name != "" {
			sourceDir = services.SanitizeDirectoryName(image.Source.Name)
		} else {
			var source models.Source
			if err := database.DB.First(&source, *image.SourceID).Error; err == nil {
				sourceDir = services.SanitizeDirectoryName(source.Name)
			}
		}
	} else if len(image.Galleries) > 0 {
		// For images, use gallery source
		if image.Galleries[0].Source != nil {
			sourceDir = services.SanitizeDirectoryName(image.Galleries[0].Source.Name)
		} else if image.Galleries[0].SourceID != nil {
			var source models.Source
			if err := database.DB.First(&source, *image.Galleries[0].SourceID).Error; err == nil {
				sourceDir = services.SanitizeDirectoryName(source.Name)
			}
		}
	}

	baseFilename := filepath.Base(image.Filename)

	// Handle video deletion
	if image.Type == "video" {
		// Delete video file
		videoPath := filepath.Join(services.UploadsDir, sourceDir, baseFilename)

		// Fallback to direct check if not found
		if _, err := os.Stat(videoPath); os.IsNotExist(err) {
			directPath := filepath.Join(services.UploadsDir, image.Filename)
			if _, err := os.Stat(directPath); err == nil {
				videoPath = directPath
			}
		}

		if err := services.DeleteFile(videoPath); err != nil {
			println("Warning: Failed to delete video file:", err.Error())
		}

		// Delete video thumbnail (video files have .jpg appended)
		thumbnailFilename := baseFilename + ".jpg"
		thumbnailPath := filepath.Join(services.UploadsDir, sourceDir, "thumbnails", thumbnailFilename)
		if err := services.DeleteFile(thumbnailPath); err != nil {
			println("Warning: Failed to delete video thumbnail:", err.Error())
		}

		// Delete trickplay files
		baseNameWithoutExt := strings.TrimSuffix(baseFilename, filepath.Ext(baseFilename))

		// Delete trickplay VTT
		trickplayVTT := filepath.Join(services.UploadsDir, sourceDir, "trickplay", baseNameWithoutExt+".vtt")
		if err := services.DeleteFile(trickplayVTT); err != nil {
			println("Warning: Failed to delete trickplay VTT:", err.Error())
		}

		// Delete trickplay sprite
		trickplaySprite := filepath.Join(services.UploadsDir, sourceDir, "trickplay", baseNameWithoutExt+"_sprite.jpg")
		if err := services.DeleteFile(trickplaySprite); err != nil {
			println("Warning: Failed to delete trickplay sprite:", err.Error())
		}
	} else {
		// Handle image deletion (original logic)
		imagePath := filepath.Join(services.UploadsDir, sourceDir, baseFilename)

		// Fallback to direct check if not found
		if _, err := os.Stat(imagePath); os.IsNotExist(err) {
			directPath := filepath.Join(services.UploadsDir, image.Filename)
			if _, err := os.Stat(directPath); err == nil {
				imagePath = directPath
			}
		}

		if err := services.DeleteFile(imagePath); err != nil {
			println("Warning: Failed to delete image file:", err.Error())
		}

		// Delete image thumbnail
		thumbnailPath := filepath.Join(services.UploadsDir, sourceDir, "thumbnails", baseFilename)
		if err := services.DeleteFile(thumbnailPath); err != nil {
			println("Warning: Failed to delete thumbnail:", err.Error())
		}
	}

	// Delete from database
	if err := database.DB.Delete(&image).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete image"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Image deleted successfully"})
}

// GetFailedImages lists images where the file is missing on disk (for dashboard)
func GetFailedImages(c *gin.Context) {
	// Use keyset pagination internally to avoid loading everything
	const pageSize = 500
	var missing []models.Image
	var lastID uint
	for {
		var page []models.Image
		if err := database.DB.Select("id, filename").Where("id > ?", lastID).Order("id ASC").Limit(pageSize).Find(&page).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query images"})
			return
		}
		if len(page) == 0 {
			break
		}
		for _, img := range page {
			fullPath := filepath.Join(services.UploadsDir, img.Filename)
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				missing = append(missing, img)
			}
		}
		lastID = page[len(page)-1].ID
	}

	c.JSON(http.StatusOK, gin.H{"data": missing})
}

// RetryImage attempts to re-download a single image by ID
func RetryImage(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid image ID"})
		return
	}

	go func(imageID int) {
		// Fire-and-forget: use service to recover
		if err := services.RecoverImage(uint(imageID)); err != nil {
			logger.Warnf("RetryImage failed for %d: %v", imageID, err)
		}
	}(id)

	c.JSON(http.StatusAccepted, gin.H{"message": "Retry scheduled"})
}

// RetryAllImages schedules recovery for all missing images found
func RetryAllImages(c *gin.Context) {
	const pageSize = 500
	const maxRetries = 5000
	count := 0
	var lastID uint
	for {
		var page []models.Image
		if err := database.DB.Select("id, filename").Where("id > ?", lastID).Order("id ASC").Limit(pageSize).Find(&page).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query images"})
			return
		}
		if len(page) == 0 {
			break
		}
		for _, img := range page {
			fullPath := filepath.Join(services.UploadsDir, img.Filename)
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				count++
				if count > maxRetries {
					break
				}
				go func(id uint) {
					if err := services.RecoverImage(id); err != nil {
						logger.Warnf("RetryAllImages failed for %d: %v", id, err)
					}
				}(img.ID)
			}
		}
		if count > maxRetries {
			break
		}
		lastID = page[len(page)-1].ID
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "Retries scheduled", "count": count})
}

// ToggleFavorite toggles the favorite status of an image
func ToggleFavorite(c *gin.Context) {
	idStr := c.Param("imageId")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid image ID"})
		return
	}

	var image models.Image
	if err := database.DB.First(&image, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
		return
	}

    // Toggle favorite WITHOUT touching UpdatedAt so we don't affect list ordering.
    // Use UpdateColumn which updates the given column directly and does not
    // modify the model's timestamps or run hooks that would change UpdatedAt.
    newFav := !image.IsFavorite
    if err := database.DB.Model(&image).UpdateColumn("is_favorite", newFav).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update image"})
        return
    }
    image.IsFavorite = newFav

	c.JSON(http.StatusOK, gin.H{
		"message":     "Favorite status updated",
		"is_favorite": image.IsFavorite,
	})
}

// UpdateImageVrMode updates the VR mode for a video
func UpdateImageVrMode(c *gin.Context) {
	idStr := c.Param("imageId")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid image ID"})
		return
	}

	var req struct {
		VRMode string `json:"vr_mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var image models.Image
	if err := database.DB.First(&image, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Video not found"})
		return
	}

	image.VRMode = req.VRMode
	if err := database.DB.Save(&image).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update VR mode"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "VR mode updated",
		"vr_mode": image.VRMode,
	})
}
