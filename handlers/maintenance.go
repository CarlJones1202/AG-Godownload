package handlers

import (
    "net/http"
    "strings"
    "strconv"

    "gallery_api/database"
    "gallery_api/models"
    "gallery_api/config"
    "github.com/gin-gonic/gin"
)

// CleanupDupes performs a safety pass to remove duplicate images across all galleries.
// It deduplicates by (gallery_id, original_url) and by (gallery_id, filename).
// Access is gated by MAINTENANCE_TOKEN environment variable via the X-Maintenance-Token header.
func CleanupDupes(c *gin.Context) {
    // Simple token check for privileged access
    token := c.GetHeader("X-Maintenance-Token")
    if config.Global.MaintenanceToken != "" {
        if strings.TrimSpace(token) != strings.TrimSpace(config.Global.MaintenanceToken) {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
            return
        }
    }

    // Gather all images
    var images []models.Image
    if err := database.DB.Find(&images).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query images"})
        return
    }

    // Build duplicate sets
    seenURL := make(map[string]bool)
    seenFile := make(map[string]bool)
    dupByURL := make(map[uint]bool) // image IDs to delete due to URL dupes
    dupByFile := make(map[uint]bool)
    // Count totals
    total := len(images)
    var urlDupes, fileDupes int

    for _, img := range images {
        // URL-based dedup
        if img.OriginalURL != "" {
            key := img.GalleryID
            urlKey := strings.Join([]string{intToString(key), img.OriginalURL}, "|")
            if seenURL[urlKey] {
                dupByURL[img.ID] = true
                urlDupes++
            } else {
                seenURL[urlKey] = true
            }
        }
        // Filename-based dedup within the same gallery
        if img.Filename != "" {
            fkey := strings.Join([]string{intToString(img.GalleryID), img.Filename}, "|")
            if seenFile[fkey] {
                dupByFile[img.ID] = true
                fileDupes++
            } else {
                seenFile[fkey] = true
            }
        }
    }

    // Union of duplicates
    delSet := make(map[uint]bool)
    for id := range dupByURL {
        delSet[id] = true
    }
    for id := range dupByFile {
        delSet[id] = true
    }
    if len(delSet) > 0 {
        var ids []uint
        for id := range delSet {
            ids = append(ids, id)
        }
        // Delete in a single batch
        if err := database.DB.Delete(&models.Image{}, ids).Error; err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete duplicates"})
            return
        }
    }

    c.JSON(http.StatusOK, gin.H{
        "total_images": total,
        "deleted":     len(delSet),
        "url_duplicates":  urlDupes,
        "filename_duplicates": fileDupes,
        "note": "Duplicates cleanup completed. This is a maintenance operation and should be idempotent on subsequent runs.",
    })
}

// helper to convert int to string without importing strconv just for brevity here
func intToString(n uint) string {
    return strconv.Itoa(int(n))
}
