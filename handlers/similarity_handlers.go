package handlers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetSimilarImages returns ranked recommendations for a seed image, or for a
// group of seed ids (?ids=<base64 JSON array>). Blend = embedding + tags +
// colors + taste profile.
func GetSimilarImages(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "24"))
	if limit < 1 || limit > 200 {
		limit = 24
	}

	var seeds []uint
	if idStr := c.Param("id"); idStr != "" {
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid image ID"})
			return
		}
		seeds = []uint{uint(id)}
	} else if idsParam := c.Query("ids"); idsParam != "" {
		ids, err := decodeSeedIDs(idsParam)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		seeds = ids
	}
	if len(seeds) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id or ids parameter is required"})
		return
	}

	seed := seeds[0]
	recs, err := services.RecommendSimilar(seed, limit)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, services.ErrSeedNotEmbedded) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	images := make([]models.Image, 0, len(recs))
	for _, r := range recs {
		images = append(images, r.Image)
	}
	populateImagePaths(images)

	items := make([]gin.H, 0, len(recs))
	for _, r := range recs {
		im := r.Image
		items = append(items, gin.H{
			"id":             im.ID,
			"filename":       im.Filename,
			"similarity":     roundScore(r.Score),
			"reasons":        r.Reasons,
			"web_path":       im.WebPath,
			"thumbnail_path": im.ThumbnailPath,
			"favorite":       im.IsFavorite,
		})
	}

	var seedImage models.Image
	if err := database.DB.Select("id, filename, favorite").First(&seedImage, seed).Error; err != nil {
		seedImage = models.Image{ID: seed}
	}
	_, seedErr := services.LoadVector(seed)
	populateImagePaths([]models.Image{seedImage})

	nLikes, nDislikes := services.DefaultProfile.Counts()

	c.JSON(http.StatusOK, gin.H{
		"seed": gin.H{
			"id":             seedImage.ID,
			"filename":       seedImage.Filename,
			"tags":           services.ImageTagNames(seed),
			"web_path":       seedImage.WebPath,
			"thumbnail_path": seedImage.ThumbnailPath,
			"embedded":       seedErr == nil,
		},
		"profile": gin.H{
			"n_likes":    nLikes,
			"n_dislikes": nDislikes,
			"ready":      services.DefaultProfile.Ready(),
		},
		"data": items,
	})
}

// SetImageRating stores a 1-5 star rating for an image and updates the taste profile.
func SetImageRating(c *gin.Context) {
	id, ok := parseImageID(c)
	if !ok {
		return
	}
	var req struct {
		Rating int `json:"rating"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Rating < 1 || req.Rating > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rating must be between 1 and 5"})
		return
	}
	var image models.Image
	if err := database.DB.Select("id").First(&image, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
		return
	}
	if err := services.DefaultProfile.SetRating(id, req.Rating); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	nLikes, nDislikes := services.DefaultProfile.Counts()
	c.JSON(http.StatusOK, gin.H{
		"rating":  req.Rating,
		"profile": gin.H{"n_likes": nLikes, "n_dislikes": nDislikes, "ready": services.DefaultProfile.Ready()},
	})
}

// GetImageRating returns the stored rating (0 when unrated).
func GetImageRating(c *gin.Context) {
	id, ok := parseImageID(c)
	if !ok {
		return
	}
	r, err := services.DefaultProfile.GetRating(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, gin.H{"rating": 0})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rating": r})
}

// ClearImageRating removes the rating for an image.
func ClearImageRating(c *gin.Context) {
	id, ok := parseImageID(c)
	if !ok {
		return
	}
	if err := services.DefaultProfile.ClearRating(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rating": 0})
}

// ResetProfile wipes all ratings and the taste profile.
func ResetProfile(c *gin.Context) {
	if err := services.DefaultProfile.ResetAll(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Taste profile reset"})
}

// GetEmbedStatus reports embedding pipeline progress.
func GetEmbedStatus(c *gin.Context) {
	var total, embedded, pending, failed int64
	database.DB.Model(&models.Image{}).
		Where("type = ? AND file_exists = ? AND deleted_at IS NULL", "image", true).Count(&total)
	database.DB.Model(&models.ImageEmbedding{}).Count(&embedded)
	database.DB.Model(&models.EmbedQueue{}).Where("status = ?", "pending").Count(&pending)
	database.DB.Model(&models.EmbedQueue{}).Where("status = ?", "failed").Count(&failed)
	embedder := services.CurrentEmbedder()
	c.JSON(http.StatusOK, gin.H{
		"total_images": total,
		"embedded":     embedded,
		"pending":      pending,
		"failed":       failed,
		"index_size":   services.VectorIndex.Len(),
		"embedder":     embedder.Name(),
		"dimension":    embedder.Dim(),
	})
}

// BackfillEmbeddings enqueues embedding work for every unindexed image.
func BackfillEmbeddings(c *gin.Context) {
	n := services.EnqueueMissingEmbeddings()
	c.JSON(http.StatusAccepted, gin.H{"enqueued": n})
}

func parseImageID(c *gin.Context) (uint, bool) {
	id, err := strconv.Atoi(c.Param("imageId"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid image ID"})
		return 0, false
	}
	return uint(id), true
}

// decodeSeedIDs accepts a base64-encoded JSON array of ints, a base64-encoded
// comma-separated list, or a plain comma/space separated list.
func decodeSeedIDs(raw string) ([]uint, error) {
	decoded := raw
	if b, err := base64.StdEncoding.DecodeString(raw); err == nil && len(b) > 0 {
		decoded = string(b)
	}
	decoded = strings.TrimSpace(decoded)
	if strings.HasPrefix(decoded, "[") {
		var nums []int
		if err := json.Unmarshal([]byte(decoded), &nums); err == nil && len(nums) > 0 {
			out := make([]uint, 0, len(nums))
			for _, n := range nums {
				if n > 0 {
					out = append(out, uint(n))
				}
			}
			if len(out) > 0 {
				return out, nil
			}
		}
	}
	var ids []uint
	for _, part := range strings.FieldsFunc(decoded, func(r rune) bool { return r == ',' || r == ' ' }) {
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid image id list")
		}
		ids = append(ids, uint(n))
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("empty image id list")
	}
	return ids, nil
}

// populateImagePaths resolves /images/... web and thumbnail URLs from the
// gallery → source layout for a batch of images.
func populateImagePaths(images []models.Image) {
	if len(images) == 0 {
		return
	}
	ids := make([]uint, 0, len(images))
	for _, im := range images {
		ids = append(ids, im.ID)
	}
	var rows []struct {
		ImageID    uint
		SourceName string
	}
	database.DB.Table("image_galleries").
		Select("image_galleries.image_id AS image_id, sources.name AS source_name").
		Joins("JOIN galleries ON galleries.id = image_galleries.gallery_id").
		Joins("LEFT JOIN sources ON sources.id = galleries.source_id").
		Where("image_galleries.image_id IN ?", ids).
		Scan(&rows)
	sourceByImage := map[uint]string{}
	for _, r := range rows {
		if _, ok := sourceByImage[r.ImageID]; !ok {
			name := r.SourceName
			if name == "" {
				name = "uncategorized"
			}
			sourceByImage[r.ImageID] = name
		}
	}
	for i := range images {
		sourceName := sourceByImage[images[i].ID]
		if sourceName == "" {
			sourceName = "uncategorized"
		}
		sanitized := services.SanitizeDirectoryName(sourceName)
		images[i].WebPath = fmt.Sprintf("/images/%s/%s", sanitized, images[i].Filename)
		images[i].ThumbnailPath = fmt.Sprintf("/images/%s/thumbnails/%s", sanitized, images[i].Filename)
	}
}

func roundScore(v float64) float64 {
	return math.Round(v*1000) / 1000
}
