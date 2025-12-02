package services

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ColorSimilarity represents how similar two colors are (0-100, lower is more similar)
type ColorSimilarity struct {
	ImageID    uint
	Color      string
	Distance   float64
	Similarity float64 // 0-100, higher is more similar
}

// HexToRGB converts a hex color string to RGB values
func HexToRGB(hex string) (r, g, b int, err error) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0, 0, 0, fmt.Errorf("invalid hex color: %s", hex)
	}

	rgb, err := strconv.ParseInt(hex, 16, 32)
	if err != nil {
		return 0, 0, 0, err
	}

	r = int((rgb >> 16) & 0xFF)
	g = int((rgb >> 8) & 0xFF)
	b = int(rgb & 0xFF)
	return r, g, b, nil
}

// ColorDistance calculates the Euclidean distance between two RGB colors
func ColorDistance(r1, g1, b1, r2, g2, b2 int) float64 {
	dr := float64(r1 - r2)
	dg := float64(g1 - g2)
	db := float64(b1 - b2)
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

// FindSimilarColorInPalette finds the most similar color in a palette to the target color
func FindSimilarColorInPalette(targetHex string, paletteJSON string) (float64, string, error) {
	// Parse target color
	tr, tg, tb, err := HexToRGB(targetHex)
	if err != nil {
		return 0, "", err
	}

	// Parse palette
	var palette []string
	if err := json.Unmarshal([]byte(paletteJSON), &palette); err != nil {
		return 0, "", err
	}

	if len(palette) == 0 {
		return 0, "", fmt.Errorf("empty palette")
	}

	// Find closest color in palette
	minDistance := math.MaxFloat64
	closestColor := ""

	for _, colorHex := range palette {
		pr, pg, pb, err := HexToRGB(colorHex)
		if err != nil {
			continue
		}

		distance := ColorDistance(tr, tg, tb, pr, pg, pb)
		if distance < minDistance {
			minDistance = distance
			closestColor = colorHex
		}
	}

	return minDistance, closestColor, nil
}

// CalculateColorSimilarity converts distance to a 0-100 similarity score
// Distance ranges from 0 (identical) to ~441 (max RGB distance)
// We map this to 100 (identical) to 0 (completely different)
func CalculateColorSimilarity(distance float64) float64 {
	maxDistance := 441.67 // sqrt(255^2 + 255^2 + 255^2)
	similarity := (1 - (distance / maxDistance)) * 100
	if similarity < 0 {
		similarity = 0
	}
	return similarity
}
