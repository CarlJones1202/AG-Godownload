package handlers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"math"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetSimilarImages returns ranked recommendations for a seed image, for a
// group of seed ids (?ids=<base64 JSON array>), or — when neither is given —
// for the user's favorited images, finding the common ground between them.
// Blend = embedding + tags + colors + taste profile.
func GetSimilarImages(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "24"))
	if limit < 1 || limit > 200 {
		limit = 24
	}

	mode := "single"
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
		if len(seeds) > 1 {
			mode = "multi"
		}
	} else {
		// No seed provided: recommend from the favorited images by
		// consensus.
		mode = "favorites"
		var err error
		seeds, err = favoriteSeedIDs()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if len(seeds) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "no favorites yet — favorite some images first"})
			return
		}
	}

	recs, consensus, err := services.RecommendFromSeeds(seeds, limit)
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
	for i, r := range recs {
		im := images[i]
		items = append(items, gin.H{
			"id":             im.ID,
			"filename":       im.Filename,
			"similarity":     roundScore(r.Score),
			"reasons":        r.Reasons,
			"web_path":       im.WebPath,
			"thumbnail_path": im.ThumbnailPath,
			"favorite":       im.IsFavorite,
			"gallery_id":     r.GalleryID,
		})
	}

	// Seed metadata: the primary seed (for single mode back-compat) plus the
	// full embedded/skipped breakdown for the consensus display.
	_, vectorErr := services.LoadVector(seeds[0])
	seed := loadSeedImage(seeds[0])

	seedsOut := make([]gin.H, 0, len(seeds))
	seedImages := make([]models.Image, 0, len(seeds))
	for _, id := range seeds {
		seedImages = append(seedImages, models.Image{ID: id})
	}
	populateImagePaths(seedImages)
	skipped := map[uint]bool{}
	for _, id := range consensus.SkippedIDs {
		skipped[id] = true
	}
	for _, im := range seedImages {
		seedsOut = append(seedsOut, gin.H{
			"id":             im.ID,
			"web_path":       im.WebPath,
			"thumbnail_path": im.ThumbnailPath,
			"embedded":       !skipped[im.ID],
		})
	}
	seedSlice := []models.Image{seed}
	populateImagePaths(seedSlice)
	seed = seedSlice[0]

	nLikes, nDislikes := services.DefaultProfile.Counts()

	c.JSON(http.StatusOK, gin.H{
		"mode": mode,
		"seed": gin.H{
			"id":             seed.ID,
			"filename":       seed.Filename,
			"tags":           services.ImageTagNames(seeds[0]),
			"web_path":       seed.WebPath,
			"thumbnail_path": seed.ThumbnailPath,
			"embedded":       vectorErr == nil,
		},
		// All seeds with embedding status, so the UI can show the
		// favorites the consensus was built from.
		"seeds":        seedsOut,
		"common_tags":  consensus.CommonTagNames,
		"profile": gin.H{
			"n_likes":    nLikes,
			"n_dislikes": nDislikes,
			"ready":      services.DefaultProfile.Ready(),
		},
		// Base64-encoded JSON array of the recommended ids so the result set
		// can be shared / deep-linked via GET /similar?ids=<this value>.
		"ids_b64": encodeSeedIDs(recs),
		"data":    items,
	})
}

// favoriteSeedIDs returns the most recent favorited, existing image ids, most
// recent first, capped by config — these seed the favorites-consensus mode.
func favoriteSeedIDs() ([]uint, error) {
	var ids []uint
	err := database.DB.Model(&models.Image{}).
		Where("is_favorite = ? AND type = ? AND file_exists = ?", true, "image", true).
		Order("id DESC").
		Limit(config.Global.Embedding.FavoriteSeedCap).
		Pluck("id", &ids).Error
	return ids, err
}

// loadSeedImage fetches the light fields for a seed image, tolerating a
// missing row (returns a stub with just the id).
func loadSeedImage(id uint) models.Image {
	var im models.Image
	if err := database.DB.Select("id, filename, is_favorite").First(&im, id).Error; err != nil {
		return models.Image{ID: id}
	}
	return im
}

// encodeSeedIDs returns base64(json([...image ids])) for the recommendations,
// in ranked order, so the set can be passed straight back to /similar?ids=.
func encodeSeedIDs(recs []services.Recommendation) string {
	ids := make([]int, 0, len(recs))
	for _, r := range recs {
		ids = append(ids, int(r.Image.ID))
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
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
	var total, embedded, pending, failed, deferred int64
	database.DB.Model(&models.Image{}).
		Where("type = ? AND file_exists = ? AND deleted_at IS NULL", "image", true).Count(&total)
	database.DB.Model(&models.ImageEmbedding{}).Count(&embedded)
	database.DB.Model(&models.EmbedQueue{}).Where("status = ?", "pending").Count(&pending)
	database.DB.Model(&models.EmbedQueue{}).Where("status = ?", "failed").Count(&failed)
	database.DB.Model(&models.EmbedQueue{}).Where("status = ?", "deferred").Count(&deferred)
	embedder := services.CurrentEmbedder()
	c.JSON(http.StatusOK, gin.H{
		"total_images": total,
		"embedded":     embedded,
		"pending":      pending,
		"failed":       failed,
		"deferred":     deferred,
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
		// Filename may already be a relative path like "SourceName/hash.jpg"
		// (see crawler_service relPath). Use just the basename so the source
		// dir isn't doubled in the URL, which would 404.
		leaf := filepath.Base(images[i].Filename)
		images[i].WebPath = fmt.Sprintf("/images/%s/%s", sanitized, leaf)
		images[i].ThumbnailPath = fmt.Sprintf("/images/%s/thumbnails/%s", sanitized, leaf)
	}
}

func roundScore(v float64) float64 {
	return math.Round(v*1000) / 1000
}
