package handlers

import (
	"encoding/json"
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetDashboardStats returns aggregate counts for the dashboard
func GetDashboardStats(c *gin.Context) {
	var sourceCount int64
	database.DB.Model(&models.Source{}).Count(&sourceCount)

	var galleryCount int64
	database.DB.Model(&models.Gallery{}).Count(&galleryCount)

	var imageCount int64
	database.DB.Model(&models.Image{}).Where("type != ?", "video").Count(&imageCount)

	var videoCount int64
	database.DB.Model(&models.Image{}).Where("type = ?", "video").Count(&videoCount)

	var personCount int64
	database.DB.Model(&models.Person{}).Count(&personCount)

	c.JSON(http.StatusOK, gin.H{
		"sources":   sourceCount,
		"galleries": galleryCount,
		"images":    imageCount,
		"videos":    videoCount,
		"people":    personCount,
		"downloads": services.GetGlobalDownloadStatus(),
		"attention": getAttentionCounts(),
	})
}

// getAttentionCounts computes lightweight "needs attention" counts for the
// dashboard badge and overview strip. All queries are indexed except the
// missing-galleries scan, which uses the person_scan_queue composite index.
func getAttentionCounts() gin.H {
	var missingImages, missingVideos int64
	database.DB.Model(&models.Image{}).Where("type != ? AND file_exists = ?", "video", false).Count(&missingImages)
	database.DB.Model(&models.Image{}).Where("type = ? AND file_exists = ?", "video", false).Count(&missingVideos)

	var failedSources int64
	database.DB.Model(&models.Source{}).Where("status = ?", "error").Count(&failedSources)

	var embedPending, embedFailed, embedDeferred int64
	database.DB.Model(&models.EmbedQueue{}).Where("status = ?", "pending").Count(&embedPending)
	database.DB.Model(&models.EmbedQueue{}).Where("status = ?", "failed").Count(&embedFailed)
	database.DB.Model(&models.EmbedQueue{}).Where("status = ?", "deferred").Count(&embedDeferred)

	return gin.H{
		"missing_galleries": countMissingGalleries(),
		"missing_images":    missingImages,
		"missing_videos":    missingVideos,
		"failed_sources":    failedSources,
		"embed_pending":     embedPending,
		"embed_failed":      embedFailed,
		"embed_deferred":    embedDeferred,
	}
}

// countMissingGalleries totals the missing-gallery entries in the latest
// completed scan per (person, provider), mirroring GetAllMissingGalleries.
// Entries hidden via "not wanted" (scan-result exclusions) are excluded.
func countMissingGalleries() int {
	type latestScan struct {
		PersonID uint
		Provider string
		Results  string
	}
	var recent []latestScan
	if err := database.DB.Raw(`
		SELECT DISTINCT ON (person_id, provider) person_id, provider, results
		FROM person_scan_queue
		WHERE status = ? AND deleted_at IS NULL AND results IS NOT NULL AND results != ''
		ORDER BY person_id, provider, id DESC
	`, models.ScanStatusCompleted).Scan(&recent).Error; err != nil {
		logger.Warnf("Failed to count missing galleries: %v", err)
		return 0
	}

	var exclusions []models.ScanResultExclusion
	if err := database.DB.Find(&exclusions).Error; err != nil {
		logger.Warnf("Failed to load scan exclusions: %v", err)
		exclusions = []models.ScanResultExclusion{}
	}
	excludedByKey := make(map[string]bool)
	for _, ex := range exclusions {
		if ex.SourceURL != "" {
			excludedByKey[fmt.Sprintf("%d|%s|%s", ex.PersonID, strings.ToLower(ex.Provider), ex.SourceURL)] = true
		}
	}

	total := 0
	for _, s := range recent {
		var results map[string]interface{}
		if err := json.Unmarshal([]byte(s.Results), &results); err != nil {
			continue
		}
		if missing, ok := results["missing_galleries"].([]interface{}); ok {
			for _, g := range missing {
				gMap, ok := g.(map[string]interface{})
				if !ok {
					continue
				}
				url, _ := gMap["url"].(string)
				if url == "" {
					continue
				}
				if excludedByKey[fmt.Sprintf("%d|%s|%s", s.PersonID, strings.ToLower(s.Provider), url)] {
					continue
				}
				total++
			}
		}
	}
	return total
}

// GetPersonStats returns statistics for a person
func GetPersonStats(c *gin.Context) {
	personID := c.Param("id")

	var person models.Person
	if err := database.DB.First(&person, personID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	// Count galleries
	galleryCount := database.DB.Model(&person).Association("Galleries").Count()

	// Count images
	// Count images: Sum of images in galleries linked to this person
	// We want to count ALL images in galleries that this person is in, not just images directly tagged with this person
	var imageCount int64
	database.DB.Model(&models.Image{}).
		Joins("JOIN person_galleries ON person_galleries.gallery_id = images.gallery_id").
		Where("person_galleries.person_id = ? AND images.type != ?", personID, "video").
		Count(&imageCount)

	// Count video galleries: Count distinct galleries that contain videos and are linked to this person
	var videoCount int64
	database.DB.Model(&models.Gallery{}).
		Joins("JOIN person_galleries ON person_galleries.gallery_id = galleries.id").
		Joins("JOIN images ON images.gallery_id = galleries.id").
		Where("person_galleries.person_id = ? AND images.type = ?", personID, "video").
		Distinct("galleries.id").
		Count(&videoCount)

	// Get most common tags (if images have tags)
	type TagCount struct {
		TagID uint   `json:"tag_id"`
		Name  string `json:"name"`
		Count int64  `json:"count"`
	}
	var topTags []TagCount
	database.DB.Raw(`
		SELECT tags.id as tag_id, tags.name, COUNT(*) as count
		FROM tags
		JOIN image_tags ON image_tags.tag_id = tags.id
		JOIN person_images ON person_images.image_id = image_tags.image_id
		WHERE person_images.person_id = ?
		GROUP BY tags.id, tags.name
		ORDER BY count DESC
		LIMIT 10
	`, personID).Scan(&topTags)

	stats := gin.H{
		"person_id":     person.ID,
		"person_name":   person.Name,
		"gallery_count": galleryCount,
		"image_count":   imageCount,
		"video_count":   videoCount,
		"top_tags":      topTags,
	}

	c.JSON(http.StatusOK, stats)
}
