package handlers

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// SearchByColor finds images with similar colors to the provided hex color
func SearchByColor(c *gin.Context) {
	colorHex := c.Query("color")
	if colorHex == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "color parameter is required"})
		return
	}

	// Validate hex color format
	if !strings.HasPrefix(colorHex, "#") {
		colorHex = "#" + colorHex
	}
	if _, _, _, err := services.HexToRGB(colorHex); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid hex color format"})
		return
	}

	// Get threshold from query params (default 30 = fairly similar)
	thresholdStr := c.DefaultQuery("threshold", "30")
	threshold, err := strconv.ParseFloat(thresholdStr, 64)
	if err != nil || threshold < 0 || threshold > 100 {
		threshold = 30
	}

	// Pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}

	// Query all images with dominant colors
	var images []models.Image
	if err := database.DB.
		Select("id, filename, dominant_colors, original_url, download_url, created_at").
		Where("dominant_colors IS NOT NULL AND dominant_colors != '' AND dominant_colors != '[]'").
		Find(&images).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query images"})
		return
	}

	// Calculate similarity for each image
	type ImageMatch struct {
		models.Image
		MatchedColor string  `json:"matched_color"`
		Similarity   float64 `json:"similarity"`
		Distance     float64 `json:"distance"`
	}

	var matches []ImageMatch
	for _, img := range images {
		distance, matchedColor, err := services.FindSimilarColorInPalette(colorHex, img.DominantColors)
		if err != nil {
			continue
		}

		similarity := services.CalculateColorSimilarity(distance)

		// Only include if similarity meets threshold
		if similarity >= threshold {
			matches = append(matches, ImageMatch{
				Image:        img,
				MatchedColor: matchedColor,
				Similarity:   similarity,
				Distance:     distance,
			})
		}
	}

	// Sort by similarity (highest first)
	for i := 0; i < len(matches)-1; i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].Similarity > matches[i].Similarity {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	// Apply pagination
	total := len(matches)
	start := (page - 1) * limit
	end := start + limit
	if start >= total {
		matches = []ImageMatch{}
	} else {
		if end > total {
			end = total
		}
		matches = matches[start:end]
	}

	// Load gallery associations and populate paths for matched images
	for i := range matches {
		// Load galleries with source
		database.DB.Model(&matches[i].Image).Preload("Galleries.Source").Association("Galleries").Find(&matches[i].Galleries)

		// Populate paths
		sourceName := "uncategorized"
		if len(matches[i].Galleries) > 0 && matches[i].Galleries[0].SourceID != nil {
			if matches[i].Galleries[0].Source != nil {
				sourceName = matches[i].Galleries[0].Source.Name
			} else {
				var source models.Source
				if err := database.DB.First(&source, *matches[i].Galleries[0].SourceID).Error; err == nil {
					sourceName = source.Name
				}
			}
		}

		sanitizedSource := services.SanitizeDirectoryName(sourceName)
		matches[i].WebPath = fmt.Sprintf("/images/%s/%s", sanitizedSource, matches[i].Filename)
		matches[i].ThumbnailPath = fmt.Sprintf("/images/%s/thumbnails/%s", sanitizedSource, matches[i].Filename)
	}

	totalPages := (total + limit - 1) / limit

	c.JSON(http.StatusOK, gin.H{
		"data": matches,
		"meta": gin.H{
			"current_page": page,
			"total_pages":  totalPages,
			"total_items":  total,
			"limit":        limit,
			"search_color": colorHex,
			"threshold":    threshold,
		},
	})
}
