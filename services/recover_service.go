package services

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"

	"github.com/disintegration/imaging"
)

// RecoverImage attempts to re-download a single image record by ID.
// It runs the work synchronously (caller may spawn a goroutine). It updates
// the image record's filename and dominant_colors on success and generates
// a thumbnail. It also publishes simple active-download status via
// download_status_service helpers.
func RecoverImage(imageID uint) error {
	var img models.Image
	if err := database.DB.Preload("Galleries").Preload("Source").First(&img, imageID).Error; err != nil {
		return fmt.Errorf("image not found: %w", err)
	}

	if img.DownloadURL == "" {
		return fmt.Errorf("no download URL for image %d", imageID)
	}

	// Resolve source name similar to verification logic
	sourceName := "uncategorized"
	if img.Type == "video" && img.SourceID != nil && img.Source != nil {
		sourceName = img.Source.Name
	} else if len(img.Galleries) > 0 {
		if img.Galleries[0].Source != nil && img.Galleries[0].Source.Name != "" {
			sourceName = img.Galleries[0].Source.Name
		} else if img.Galleries[0].SourceID != nil {
			var src models.Source
			if database.DB.Select("name").Find(&src, *img.Galleries[0].SourceID).RowsAffected > 0 {
				sourceName = src.Name
			}
		}
	}

	provider := extractImageProvider(img.DownloadURL)

	// Track active download for UI
	AddActiveVerificationDownload(img.ID, filepath.Base(img.Filename), img.DownloadURL, sourceName)
	// Ensure we remove active after work
	defer RemoveActiveVerificationDownload(img.ID)

	// Use the download URL's origin as referer
	referer := ""
	if u, err := url.Parse(img.DownloadURL); err == nil {
		referer = u.Scheme + "://" + u.Host
	}

	// Attempt primary download
	result, err := DownloadImage(img.DownloadURL, sourceName, referer)
	if err != nil {
		logger.Warnf("[%s] Single-image recovery failed for ID %d: %v", provider, img.ID, err)
		// For imx provider, optionally attempt gallery-dl fallback
		if provider == "imx" && config.Global.GalleryDL.Enabled {
			logger.Debugf("Attempting gallery-dl fallback for image ID %d", img.ID)
			// create a context with timeout
			ctx := context.Background()
			result, err = DownloadImageWithGalleryDL(ctx, img.DownloadURL, sourceName, time.Duration(config.Global.GalleryDL.TimeoutSec)*time.Second)
			if err != nil {
				logger.Warnf("[imx] gallery-dl fallback failed for ID %d: %v", img.ID, err)
				return err
			}
		} else {
			return err
		}
	}

	// Compute relative path
	relPath, rerr := filepath.Rel(UploadsDir, result.Path)
	if rerr != nil {
		relPath = filepath.Join(sourceName, filepath.Base(result.Path))
	}

	// Save DB updates
	if err := database.DB.Model(&models.Image{}).Where("id = ?", img.ID).Updates(map[string]interface{}{
		"filename":        relPath,
		"dominant_colors": result.DominantColors,
	}).Error; err != nil {
		logger.Errorf("Failed to update image %d after recovery: %v", img.ID, err)
		return err
	}

	// Generate thumbnail (best-effort)
	if _, terr := imaging.Open(result.Path); terr == nil {
		// Use existing GenerateThumbnail helper
		if _, gerr := GenerateThumbnail(result.Path); gerr != nil {
			logger.Warnf("Failed to generate thumbnail for recovered image %d: %v", img.ID, gerr)
		}
	}

	logger.Infof("Successfully recovered image %d -> %s", img.ID, relPath)
	return nil
}
