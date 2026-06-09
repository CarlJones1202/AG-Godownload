package services

import (
	"encoding/json"
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"os"
	"os/exec"
	"path/filepath"
)

// AITagResult represents the JSON output from the Python script
type AITagResult struct {
	Content []AITagItem `json:"content"`
	Pose    []AITagItem `json:"pose"`
	Mood    []AITagItem `json:"mood"`
	Vibe    []AITagItem `json:"vibe"`
	Error   string      `json:"error,omitempty"`
}

type AITagItem struct {
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
}

// LabelImage runs the AI tagging script for a given image
func LabelImage(imageID uint) error {
	var image models.Image
	if err := database.DB.Preload("Tags").First(&image, imageID).Error; err != nil {
		return fmt.Errorf("image not found: %w", err)
	}

	// Construct absolute path to image
	// Assuming image.DownloadURL or a local path is available.
	// If the image is remote, we might need to use the local file path if we have it?
	// The Image model has 'WebPath', 'ThumbnailPath'.
	// We likely need the actual file on disk.
	// Let's assume we can map WebPath back to a local path or use a sophisticated resolver.
	// For now, let's try to verify where the file actually is.
	// If it's a ripped image, it should be in 'uploads' or 'gallery' folder.
	// Let's try to resolve it relative to current working directory.

	imagePath := image.WebPath
	// If WebPath starts with /api/images/, it's a serve path.
	// We need the physical path.
	// Looking at ripper_service, images are saved to "uploads/<gallery_name>/<filename>" usually.
	// Or we can rely on verifying the file exists.

	// Quick fix: attempt to find file in typical locations if path is relative or web-path
	if len(imagePath) > 0 && imagePath[0] == '/' {
		// remove leading slash
		imagePath = imagePath[1:]
	}

	// If it doesn't exist, try decoding it (common in this app if I recall correctly)
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		// Try looking in uploads
		// This part deals with path resolution which might be tricky without more context on file_service
		// For now, let's assume imagePath is relative to CWD.
		// If it fails, the script will return error.
	}

	// Get absolute path for python script
	cwd, _ := os.Getwd()
	scriptPath := filepath.Join(cwd, "scripts", "ai_tagger.py")
	absImagePath, _ := filepath.Abs(imagePath)

	// Find a python executable. On Windows machines the "python" command
	// may be unavailable; try common alternatives (python3, py).
	var pythonExec string
	for _, candidate := range []string{"python", "python3", "py"} {
		if p, err := exec.LookPath(candidate); err == nil {
			pythonExec = p
			break
		}
	}
	if pythonExec == "" {
		return fmt.Errorf("python executable not found. Install Python or enable App Execution Aliases. Tried: python, python3, py")
	}

	cmd := exec.Command(pythonExec, scriptPath, "--image", absImagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("python script execution failed: %v, output: %s", err, string(output))
	}

	var result AITagResult
	if err := json.Unmarshal(output, &result); err != nil {
		return fmt.Errorf("failed to parse script output: %v, output: %s", err, string(output))
	}

	if result.Error != "" {
		return fmt.Errorf("script reported error: %s", result.Error)
	}

	// Transaction to save tags
	tx := database.DB.Begin()

	processCategory := func(items []AITagItem, category string) {
		for _, item := range items {
			var tag models.Tag
			// Find or create tag
			if err := tx.FirstOrCreate(&tag, models.Tag{Name: item.Name, Category: category}).Error; err != nil {
				logger.Errorf("Failed to find/create tag %s: %v", item.Name, err)
				continue
			}

			// Associate with image if not already
			if err := tx.Model(&image).Association("Tags").Append(&tag); err != nil {
				logger.Errorf("Failed to associate tag %s: %v", item.Name, err)
			}
		}
	}

	processCategory(result.Content, "content")
	processCategory(result.Pose, "pose")
	processCategory(result.Mood, "mood")
	processCategory(result.Vibe, "vibe")

	return tx.Commit().Error
}

// ScanUntaggedImages finds images with no tags and processes them
func ScanUntaggedImages() error {
	// Use LEFT JOIN instead of NOT IN subquery for SQLite performance
	// Process in chunks to limit memory usage
	const chunkSize = 500
	offset := 0
	totalAdded := 0

	for {
		var images []models.Image
		if err := database.DB.Select("id").
			Joins("LEFT JOIN image_tags ON image_tags.image_id = images.id").
			Where("image_tags.image_id IS NULL").
			Order("id ASC").
			Limit(chunkSize).Offset(offset).
			Find(&images).Error; err != nil {
			return err
		}
		if len(images) == 0 {
			break
		}

		for _, img := range images {
			AddToAITagQueue(img.ID)
			totalAdded++
		}
		offset += chunkSize
	}

	logger.Infof("Found %d untagged images for AI labeling", totalAdded)
	return nil
}
