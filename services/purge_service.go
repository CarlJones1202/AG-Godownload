package services

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// PurgePlaceholderResult summarizes what was found and removed.
type PurgePlaceholderResult struct {
	ScannedFiles     int                       `json:"scanned_files"`
	PlaceholdersFound int                      `json:"placeholders_found"`
	DeletedFromDB    int                       `json:"deleted_from_db"`
	DeletedFiles     int                       `json:"deleted_files"`
	Errors           []string                  `json:"errors,omitempty"`
	Details          []PurgePlaceholderDetail  `json:"details,omitempty"`
}

// PurgePlaceholderDetail records one removed placeholder.
type PurgePlaceholderDetail struct {
	ImageID  uint   `json:"image_id"`
	Filename string `json:"filename"`
	Reason   string `json:"reason"`
	Provider string `json:"provider"`
	Action   string `json:"action"` // "deleted" or "db_only"
}

// ScanAndPurgePlaceholders walks the uploads directory, checks every image
// against IsHotlinkPlaceholder, and removes any that match.
func ScanAndPurgePlaceholders() (*PurgePlaceholderResult, error) {
	result := &PurgePlaceholderResult{}
	var (
		mu              sync.Mutex
		wg              sync.WaitGroup
		scannedFiles    int32
		placeholdersCnt int32
		deletedFromDB   int32
		deletedFiles    int32
	)
	sem := make(chan struct{}, 10) // limit concurrent checks

	logger.Info("Starting placeholder image scan and purge...")

	// Phase 1: walk filesystem and check every image
	_, err := os.Stat(UploadsDir)
	if os.IsNotExist(err) {
		return result, fmt.Errorf("uploads directory %s does not exist", UploadsDir)
	}

	err = filepath.Walk(UploadsDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			base := filepath.Base(path)
			if base == "thumbnails" || base == "trickplay" || base == "gallery_thumbnails" || base == "person_images" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".gif" && ext != ".webp" {
			return nil
		}

		// Skip thumbnails subdirs just in case SkipDir wasn't respected
		pathSlash := filepath.ToSlash(path)
		if strings.Contains(pathSlash, "/thumbnails/") || strings.Contains(pathSlash, "\\thumbnails\\") {
			return nil
		}

		atomic.AddInt32(&scannedFiles, 1)

		wg.Add(1)
		sem <- struct{}{}
		go func(p string) {
			defer wg.Done()
			defer func() { <-sem }()

			provider := "unknown"
			if parts := strings.Split(filepath.ToSlash(p), "/"); len(parts) >= 3 {
				// e.g. uploads/SomeSource/abc.jpg -> "SomeSource"
				provider = parts[len(parts)-2]
			}

			isPlaceholder, reason, err := IsHotlinkPlaceholder(p, provider)
			if err != nil {
				logger.Warnf("Error checking %s: %v", p, err)
				return
			}
			if !isPlaceholder {
				return
			}

			detail := PurgePlaceholderDetail{
				Filename: filepath.Base(p),
				Reason:   reason,
				Provider: provider,
			}

			// Try to find matching DB records (many images may share the same filename
			// if they all resolved to the same placeholder hash)
			baseName := filepath.Base(p)
			var images []models.Image
			if err := database.DB.Where("filename LIKE ?", "%"+baseName).Find(&images).Error; err == nil {
				var matchedIDs []uint
				for _, img := range images {
					imgFullPath := filepath.Join(UploadsDir, img.Filename)
					imgAbs, _ := filepath.Abs(imgFullPath)
					pAbs, _ := filepath.Abs(p)
					if imgAbs == pAbs {
						matchedIDs = append(matchedIDs, img.ID)
					}
				}
				if len(matchedIDs) > 0 {
					// Batch-delete all matching records
					if delErr := database.DB.Unscoped().Delete(&models.Image{}, matchedIDs).Error; delErr != nil {
						logger.Warnf("Failed to batch-delete %d images from DB: %v", len(matchedIDs), delErr)
						mu.Lock()
						result.Errors = append(result.Errors, fmt.Sprintf("batch db delete failed for %d images: %v", len(matchedIDs), delErr))
						mu.Unlock()
					} else {
						atomic.AddInt32(&deletedFromDB, int32(len(matchedIDs)))
						detail.ImageID = matchedIDs[0]
						detail.Action = "deleted"
					}
					// Remove thumbnail
					thumbPath := filepath.Join(filepath.Dir(p), "thumbnails", baseName)
					os.Remove(thumbPath)
				}
			} else {
				logger.Warnf("DB lookup failed for %s: %v", baseName, err)
			}

			if detail.Action == "" {
				detail.Action = "db_only"
			}

			if rmErr := os.Remove(p); rmErr != nil && !os.IsNotExist(rmErr) {
				logger.Warnf("Failed to delete placeholder file %s: %v", p, rmErr)
				mu.Lock()
				result.Errors = append(result.Errors, fmt.Sprintf("file delete failed for %s: %v", p, rmErr))
				mu.Unlock()
			} else {
				atomic.AddInt32(&deletedFiles, 1)
			}

			atomic.AddInt32(&placeholdersCnt, 1)
			mu.Lock()
			result.Details = append(result.Details, detail)
			mu.Unlock()

			logger.Infof("Purged placeholder: %s (ID %d) — %s", p, detail.ImageID, reason)
		}(path)
		return nil
	})

	wg.Wait()

	if err != nil {
		return result, fmt.Errorf("filesystem walk failed: %w", err)
	}

	result.ScannedFiles = int(atomic.LoadInt32(&scannedFiles))
	result.PlaceholdersFound = int(atomic.LoadInt32(&placeholdersCnt))
	result.DeletedFiles = int(atomic.LoadInt32(&deletedFiles))
	result.DeletedFromDB = int(atomic.LoadInt32(&deletedFromDB))

	logger.Infof("Placeholder scan complete: %d placeholders found, %d files deleted, %d DB records removed",
		result.PlaceholdersFound, result.DeletedFiles, result.DeletedFromDB)

	return result, nil
}

// RecheckDownloadedImages scans all images in the DB that have download URLs
// pointing to known placeholder-prone providers (imagetwist, acidimg, etc.)
// and verifies they aren't placeholders. If a placeholder is found, it
// re-downloads the image with the correct referer.
func RecheckDownloadedImages(providerFilter string) error {
	logger.Infof("Rechecking downloaded images for provider: %s", providerFilter)

	// Build provider URL filter
	var likeClauses []string
	switch providerFilter {
	case "imagetwist":
		likeClauses = []string{"%imagetwist%"}
	case "all":
		likeClauses = []string{"%imagetwist%", "%acidimg%", "%imx.to%"}
	default:
		if providerFilter != "" {
			likeClauses = []string{"%" + providerFilter + "%"}
		}
	}

	if len(likeClauses) == 0 {
		return fmt.Errorf("no provider filter specified")
	}

	const chunkSize = 100
	var lastID uint
	checkedCount := 0
	replacedCount := 0

	for {
		query := database.DB.Model(&models.Image{}).
			Where("id > ?", lastID).
			Order("id ASC").
			Limit(chunkSize)

		if len(likeClauses) > 0 {
			likeParams := make([]interface{}, len(likeClauses))
			for i, lc := range likeClauses {
				likeParams[i] = lc
			}
			query = query.Where("(download_url LIKE ?)"+strings.Repeat(" OR download_url LIKE ?", len(likeClauses)-1), likeParams...)
		}

		var images []models.Image
		if err := query.Find(&images).Error; err != nil {
			return fmt.Errorf("query failed: %w", err)
		}
		if len(images) == 0 {
			break
		}

		for _, img := range images {
			fullPath := filepath.Join(UploadsDir, img.Filename)
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				continue
			}

			provider := extractImageProvider(img.DownloadURL)
			isPlaceholder, reason, err := IsHotlinkPlaceholder(fullPath, provider)
			if err != nil {
				logger.Warnf("Error checking image %d: %v", img.ID, err)
				continue
			}
			checkedCount++

			if isPlaceholder {
				logger.Infof("Image %d (%s) IS a placeholder: %s — re-downloading", img.ID, img.Filename, reason)

				// Delete the bad file
				os.Remove(fullPath)
				thumbPath := filepath.Join(filepath.Dir(fullPath), "thumbnails", filepath.Base(fullPath))
				os.Remove(thumbPath)

				// Re-download with corrected referer
				sourceName := "uncategorized"
				if img.SourceID != nil {
					var src models.Source
					if err := database.DB.Select("name").First(&src, *img.SourceID).Error; err == nil {
						sourceName = src.Name
					}
				} else {
					var gallery models.Gallery
					if err := database.DB.Joins("JOIN image_galleries ON image_galleries.gallery_id = galleries.id").
						Where("image_galleries.image_id = ?", img.ID).First(&gallery).Error; err == nil {
						if gallery.SourceID != nil {
							var src models.Source
							if err := database.DB.Select("name").First(&src, *gallery.SourceID).Error; err == nil {
								sourceName = src.Name
							}
						}
					}
				}

				// Use the original page URL as referer if it's an imagetwist image
				referer := img.OriginalURL
				if referer == "" {
					referer = img.DownloadURL
				}

				result, dlErr := DownloadImage(img.DownloadURL, sourceName, referer)
				if dlErr != nil {
					logger.Warnf("Re-download failed for image %d: %v", img.ID, dlErr)
					continue
				}

				relPath, _ := filepath.Rel(UploadsDir, result.Path)
				if err := database.DB.Model(&img).Updates(map[string]interface{}{
					"filename":        relPath,
					"dominant_colors": result.DominantColors,
				}).Error; err != nil {
					logger.Errorf("Failed to update image %d after re-download: %v", img.ID, err)
					continue
				}
				GenerateThumbnail(result.Path)
				replacedCount++
				logger.Infof("Re-downloaded and replaced placeholder image %d -> %s", img.ID, relPath)
			}
		}

		lastID = images[len(images)-1].ID

		// Polite pause between chunks
		time.Sleep(500 * time.Millisecond)
	}

	logger.Infof("Recheck complete: %d images checked, %d replaced", checkedCount, replacedCount)
	return nil
}
