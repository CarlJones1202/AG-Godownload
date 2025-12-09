package services

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"os"
	"path/filepath"
)

// IsLocalPath checks if the string looks like a file path
func IsLocalPath(path string) bool {
	// Simple check: absolute path or file existence
	if filepath.IsAbs(path) {
		return true
	}
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
}

// ProcessLocalSource handles importing a local file as a source item
func ProcessLocalSource(source models.Source) error {
	path := source.Location

	if IsVideoFile(path) {
		logger.Infof("Importing video file: %s", path)
		result, err := ImportLocalVideo(path, source.Name)
		if err != nil {
			return fmt.Errorf("failed to import video: %w", err)
		}

		// For videos, create image record without gallery but with direct source reference
		image := models.Image{
			SourceID:       &source.ID,
			Filename:       filepath.Base(result.Path),
			OriginalURL:    fmt.Sprintf("file://%s", path),
			DownloadURL:    fmt.Sprintf("file://%s", result.Path),
			DominantColors: "[]",
			Type:           "video",
		}

		if err := database.DB.Create(&image).Error; err != nil {
			return fmt.Errorf("failed to save video record: %w", err)
		}
		logger.Infof("Successfully imported video")

	} else {
		// For images, create gallery first
		var gallery models.Gallery
		if err := database.DB.Where("source_id = ?", source.ID).First(&gallery).Error; err != nil {
			gallery = models.Gallery{
				Name:     source.Name,
				SourceID: &source.ID,
			}
			if err := database.DB.Create(&gallery).Error; err != nil {
				return err
			}
		}

		logger.Infof("Importing local image file: %s", path)

		result, err := ImportLocalVideo(path, source.Name)
		if err != nil {
			return err
		}

		image := models.Image{
			GalleryID:      gallery.ID,
			Filename:       filepath.Base(result.Path),
			OriginalURL:    fmt.Sprintf("file://%s", path),
			DownloadURL:    fmt.Sprintf("file://%s", result.Path),
			DominantColors: result.DominantColors,
			Type:           "image",
			Galleries:      []*models.Gallery{&gallery},
		}

		// If it's an image, we should try to extract colors/thumb
		if _, err := GenerateThumbnail(result.Path); err != nil {
			logger.Warn("Failed to generate thumbnail for local image")
		}
		if colors, err := ExtractDominantColors(result.Path); err == nil {
			image.DominantColors = colors
		}

		if err := database.DB.Create(&image).Error; err != nil {
			return err
		}
	}

	database.DB.Model(&source).Update("Status", "idle")
	return nil
}
