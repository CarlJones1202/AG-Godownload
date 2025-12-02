package services

import (
	"encoding/json"
	"fmt"
	"image"
	"math"
	"sort"

	"github.com/disintegration/imaging"
)

const (
	// Number of dominant colors to extract
	NumDominantColors = 5
	// Number of k-means iterations
	KMeansIterations = 10
	// Sample size for color extraction (to improve performance)
	MaxSamplePixels = 10000
)

// Color represents an RGB color with frequency
type Color struct {
	R, G, B   uint8
	Frequency int
}

// ToHex converts a Color to hex string format
func (c Color) ToHex() string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

// ExtractDominantColors extracts the most common colors from an image file
// Returns a JSON string containing an array of hex color codes
func ExtractDominantColors(imagePath string) (string, error) {
	// Load the image
	img, err := imaging.Open(imagePath)
	if err != nil {
		return "", fmt.Errorf("failed to open image: %w", err)
	}

	// Extract colors using k-means clustering
	colors := extractColorsKMeans(img, NumDominantColors)

	// Convert to hex strings
	hexColors := make([]string, len(colors))
	for i, c := range colors {
		hexColors[i] = c.ToHex()
	}

	// Convert to JSON
	jsonData, err := json.Marshal(hexColors)
	if err != nil {
		return "", fmt.Errorf("failed to marshal colors: %w", err)
	}

	return string(jsonData), nil
}

// extractColorsKMeans uses k-means clustering to find dominant colors
func extractColorsKMeans(img image.Image, k int) []Color {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	totalPixels := width * height

	// Sample pixels if image is too large
	step := 1
	if totalPixels > MaxSamplePixels {
		step = int(math.Sqrt(float64(totalPixels) / float64(MaxSamplePixels)))
	}

	// Collect pixel colors
	var pixels []Color
	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			c := img.At(x, y)
			r, g, b, a := c.RGBA()
			// Skip transparent pixels
			if a < 32768 { // Less than 50% opacity
				continue
			}
			pixels = append(pixels, Color{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(b >> 8),
			})
		}
	}

	if len(pixels) == 0 {
		return []Color{}
	}

	// Initialize centroids with random pixels
	centroids := make([]Color, k)
	step = len(pixels) / k
	for i := 0; i < k && i*step < len(pixels); i++ {
		centroids[i] = pixels[i*step]
	}

	// K-means clustering
	for iter := 0; iter < KMeansIterations; iter++ {
		// Assign pixels to nearest centroid
		clusters := make([][]Color, k)
		for _, pixel := range pixels {
			nearestIdx := 0
			minDist := colorDistance(pixel, centroids[0])
			for i := 1; i < k; i++ {
				dist := colorDistance(pixel, centroids[i])
				if dist < minDist {
					minDist = dist
					nearestIdx = i
				}
			}
			clusters[nearestIdx] = append(clusters[nearestIdx], pixel)
		}

		// Update centroids
		for i := 0; i < k; i++ {
			if len(clusters[i]) > 0 {
				centroids[i] = averageColor(clusters[i])
			}
		}
	}

	// Calculate frequency for each centroid
	result := make([]Color, 0, k)
	for i := 0; i < k; i++ {
		count := 0
		for _, pixel := range pixels {
			nearestIdx := 0
			minDist := colorDistance(pixel, centroids[0])
			for j := 1; j < k; j++ {
				dist := colorDistance(pixel, centroids[j])
				if dist < minDist {
					minDist = dist
					nearestIdx = j
				}
			}
			if nearestIdx == i {
				count++
			}
		}
		if count > 0 {
			centroids[i].Frequency = count
			result = append(result, centroids[i])
		}
	}

	// Sort by frequency (most common first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Frequency > result[j].Frequency
	})

	return result
}

// colorDistance calculates the Euclidean distance between two colors
func colorDistance(c1, c2 Color) float64 {
	dr := float64(c1.R) - float64(c2.R)
	dg := float64(c1.G) - float64(c2.G)
	db := float64(c1.B) - float64(c2.B)
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

// averageColor calculates the average color from a slice of colors
func averageColor(colors []Color) Color {
	if len(colors) == 0 {
		return Color{}
	}

	var sumR, sumG, sumB int
	for _, c := range colors {
		sumR += int(c.R)
		sumG += int(c.G)
		sumB += int(c.B)
	}

	count := len(colors)
	return Color{
		R: uint8(sumR / count),
		G: uint8(sumG / count),
		B: uint8(sumB / count),
	}
}
