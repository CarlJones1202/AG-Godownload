package handlers

import "testing"

func TestAddRecommendationEventCountTracksCountsAndAverage(t *testing.T) {
	var counts recommendationMetricCounts
	addRecommendationEventCount(&counts, recommendationEventAggregate{
		Event: "impression",
		Count: 8,
	})
	addRecommendationEventCount(&counts, recommendationEventAggregate{
		Event: "open",
		Count: 3,
	})
	addRecommendationEventCount(&counts, recommendationEventAggregate{
		Event:         "rating",
		Count:         2,
		EventCount:    4,
		AverageRating: 3.5,
	})

	if counts.Impressions != 8 {
		t.Errorf("impressions = %d, want 8", counts.Impressions)
	}
	if counts.Opens != 3 {
		t.Errorf("opens = %d, want 3", counts.Opens)
	}
	if counts.Ratings != 4 {
		t.Errorf("ratings = %d, want 4", counts.Ratings)
	}
	if counts.AverageRating != 3.5 {
		t.Errorf("average rating = %f, want 3.5", counts.AverageRating)
	}
}

func TestCalculateRecommendationRatesUsesImpressions(t *testing.T) {
	counts := recommendationMetricCounts{
		Impressions:   10,
		Opens:         3,
		Favorites:     2,
		NotInterested: 1,
	}

	calculateRecommendationRates(&counts)

	if counts.OpenRate != 0.3 || counts.FavoriteRate != 0.2 || counts.DislikeRate != 0.1 {
		t.Errorf("rates = (%f, %f, %f), want (0.3, 0.2, 0.1)",
			counts.OpenRate, counts.FavoriteRate, counts.DislikeRate)
	}
}
