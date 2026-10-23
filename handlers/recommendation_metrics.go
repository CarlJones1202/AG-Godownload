package handlers

import (
	"gallery_api/database"
	"gallery_api/models"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type recommendationMetricCounts struct {
	Impressions   int64   `json:"impressions"`
	Opens         int64   `json:"opens"`
	Favorites     int64   `json:"favorites"`
	NotInterested int64   `json:"not_interested"`
	Ratings       int64   `json:"ratings"`
	AverageRating float64 `json:"average_rating"`
	OpenRate      float64 `json:"open_rate"`
	FavoriteRate  float64 `json:"favorite_rate"`
	DislikeRate   float64 `json:"not_interested_rate"`
}

type recommendationEventAggregate struct {
	Mode          string
	Event         string
	Count         int64
	EventCount    int64
	AverageRating float64
}

// GetRecommendationMetrics summarizes outcomes for delivered recommendation
// sets over the requested trailing day window (defaults to 30 days).
func GetRecommendationMetrics(c *gin.Context) {
	days, err := strconv.Atoi(c.DefaultQuery("days", "30"))
	if err != nil || days < 1 || days > 365 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "days must be between 1 and 365"})
		return
	}
	since := time.Now().AddDate(0, 0, -days)

	var rows []recommendationEventAggregate
	if err := database.DB.Model(&models.RecommendationEvent{}).
		Select("mode, event, COUNT(DISTINCT (set_id, image_id)) AS count, COUNT(*) AS event_count, COALESCE(AVG(NULLIF(rating, 0)), 0) AS average_rating").
		Where("created_at >= ?", since).
		Group("mode, event").
		Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	byMode := make(map[string]recommendationMetricCounts)
	var total recommendationMetricCounts
	for _, row := range rows {
		counts := byMode[row.Mode]
		addRecommendationEventCount(&counts, row)
		byMode[row.Mode] = counts
		addRecommendationEventCount(&total, row)
	}
	var setRows []struct {
		Mode  string
		Count int64
	}
	if err := database.DB.Model(&models.RecommendationEvent{}).
		Select("mode, COUNT(DISTINCT set_id) AS count").
		Where("created_at >= ? AND event = ?", since, "impression").
		Group("mode").
		Scan(&setRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	setsByMode := make(map[string]int64, len(setRows))
	var totalSets int64
	for _, row := range setRows {
		setsByMode[row.Mode] = row.Count
		totalSets += row.Count
	}
	calculateRecommendationRates(&total)
	for mode, counts := range byMode {
		calculateRecommendationRates(&counts)
		byMode[mode] = counts
	}

	c.JSON(http.StatusOK, gin.H{
		"days":         days,
		"since":        since,
		"sets":         totalSets,
		"sets_by_mode": setsByMode,
		"total":        total,
		"by_mode":      byMode,
	})
}

func addRecommendationEventCount(counts *recommendationMetricCounts, row recommendationEventAggregate) {
	switch row.Event {
	case "impression":
		counts.Impressions += row.Count
	case "open":
		counts.Opens += row.Count
	case "favorite":
		counts.Favorites += row.Count
	case "not_interested":
		counts.NotInterested += row.Count
	case "rating":
		counts.Ratings += row.EventCount
		weightedRating := counts.AverageRating * float64(counts.Ratings-row.EventCount)
		counts.AverageRating = (weightedRating + row.AverageRating*float64(row.EventCount)) / float64(counts.Ratings)
	}
}

func calculateRecommendationRates(counts *recommendationMetricCounts) {
	if counts.Impressions == 0 {
		return
	}
	denominator := float64(counts.Impressions)
	counts.OpenRate = float64(counts.Opens) / denominator
	counts.FavoriteRate = float64(counts.Favorites) / denominator
	counts.DislikeRate = float64(counts.NotInterested) / denominator
}
