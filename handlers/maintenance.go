package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"gallery_api/services"
	"github.com/gin-gonic/gin"
)

func authorizeMaintenance(c *gin.Context) bool {
	expectedToken := strings.TrimSpace(config.Global.MaintenanceToken)
	if expectedToken == "" || strings.TrimSpace(c.GetHeader("X-Maintenance-Token")) != expectedToken {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false
	}
	return true
}

// CleanupDupes performs a safety pass to remove duplicate images across all galleries.
// It deduplicates by (gallery_id, original_url) and by (gallery_id, filename).
// Access is gated by MAINTENANCE_TOKEN environment variable via the X-Maintenance-Token header.
func CleanupDupes(c *gin.Context) {
	if !authorizeMaintenance(c) {
		return
	}

	// Gather images in paginated chunks so we don't materialize the whole table
	// into memory. The first occurrence (lowest id) wins; later duplicates are
	// removed, matching the previous behaviour but bounded in memory.
	const pageSize = 1000
	var lastID uint
	total := 0
	var urlDupes, fileDupes int
	seenURL := make(map[string]bool)
	seenFile := make(map[string]bool)
	delSet := make(map[uint]bool)

	for {
		var page []struct {
			ID          uint
			GalleryID   uint
			OriginalURL string
			Filename    string
		}
		if err := database.DB.Model(&models.Image{}).
			Select("id, gallery_id, original_url, filename").
			Where("id > ?", lastID).
			Order("id ASC").
			Limit(pageSize).
			Scan(&page).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query images"})
			return
		}
		if len(page) == 0 {
			break
		}
		total += len(page)

		for _, img := range page {
			lastID = img.ID
			// URL-based dedup
			if img.OriginalURL != "" {
				urlKey := strings.Join([]string{intToString(img.GalleryID), img.OriginalURL}, "|")
				if seenURL[urlKey] {
					delSet[img.ID] = true
					urlDupes++
				} else {
					seenURL[urlKey] = true
				}
			}
			// Filename-based dedup within the same gallery
			if img.Filename != "" {
				fkey := strings.Join([]string{intToString(img.GalleryID), img.Filename}, "|")
				if seenFile[fkey] {
					delSet[img.ID] = true
					fileDupes++
				} else {
					seenFile[fkey] = true
				}
			}
		}
	}

	if len(delSet) > 0 {
		ids := make([]uint, 0, len(delSet))
		for id := range delSet {
			ids = append(ids, id)
		}
		// Delete duplicates in batches to avoid an overly large IN clause.
		const delBatch = 500
		for i := 0; i < len(ids); i += delBatch {
			end := i + delBatch
			if end > len(ids) {
				end = len(ids)
			}
			if err := database.DB.Delete(&models.Image{}, ids[i:end]).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete duplicates"})
				return
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total_images":        total,
		"deleted":             len(delSet),
		"url_duplicates":      urlDupes,
		"filename_duplicates": fileDupes,
		"note":                "Duplicates cleanup completed. This is a maintenance operation and should be idempotent on subsequent runs.",
	})
}

// helper to convert int to string without importing strconv just for brevity here
func intToString(n uint) string {
	return strconv.Itoa(int(n))
}

// PurgePlaceholders scans all images on disk, detects known hotlinking-placeholder
// images (e.g. "hotlinking is disabled" from imagetwist), and deletes them from
// both the filesystem and the database.  Protected by X-Maintenance-Token.
func PurgePlaceholders(c *gin.Context) {
	if !authorizeMaintenance(c) {
		return
	}

	result, err := services.ScanAndPurgePlaceholders()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// SyncFileExists triggers a disk scan to update the file_exists boolean for all images.
func SyncFileExists(c *gin.Context) {
	if !authorizeMaintenance(c) {
		return
	}
	go func() {
		if err := services.SyncFileExists(); err != nil {
			logger.Errorf("File exists synchronization failed: %v", err)
		}
	}()
	c.JSON(http.StatusOK, gin.H{"message": "File exists synchronization initiated in background"})
}
