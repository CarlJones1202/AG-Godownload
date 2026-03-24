package handlers

import (
	"encoding/json"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
)

func TriggerPersonScan(c *gin.Context) {
	idStr := c.Param("id")
	personID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid person ID"})
		return
	}

	var person models.Person
	if err := database.DB.First(&person, personID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	// Get provider aliases for this person
	var aliases []models.PersonProviderAlias
	if err := database.DB.Where("person_id = ?", personID).Find(&aliases).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch aliases"})
		return
	}

	if len(aliases) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No provider aliases defined for this person"})
		return
	}

	// Queue scans for each unique provider
	seen := make(map[string]bool)
	for _, alias := range aliases {
		if seen[alias.Provider] {
			continue
		}
		seen[alias.Provider] = true

		if err := services.AddToScanQueue(uint(personID), alias.Provider, alias.Alias); err != nil {
			logger.Warnf("Failed to queue scan for person %d, provider %s: %v", personID, alias.Provider, err)
		}
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "Scan queued", "providers": len(seen)})
}

func GetPersonScanResults(c *gin.Context) {
	idStr := c.Param("id")
	personID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid person ID"})
		return
	}

	scans, err := services.GetScanResults(uint(personID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch scan results"})
		return
	}

	type ScanResultResponse struct {
		ID          uint                   `json:"id"`
		PersonID    uint                   `json:"person_id"`
		Provider    string                 `json:"provider"`
		Alias       string                 `json:"alias"`
		Status      string                 `json:"status"`
		Error       string                 `json:"error,omitempty"`
		CreatedAt   string                 `json:"created_at"`
		StartedAt   *string                `json:"started_at,omitempty"`
		CompletedAt *string                `json:"completed_at,omitempty"`
		Results     map[string]interface{} `json:"results,omitempty"`
	}

	response := make([]ScanResultResponse, len(scans))
	for i, scan := range scans {
		r := ScanResultResponse{
			ID:       scan.ID,
			PersonID: scan.PersonID,
			Provider: scan.Provider,
			Alias:    scan.Alias,
			Status:   string(scan.Status),
			Error:    scan.Error,
		}

		r.CreatedAt = scan.CreatedAt.Format("2006-01-02T15:04:05Z")
		if scan.StartedAt != nil {
			started := scan.StartedAt.Format("2006-01-02T15:04:05Z")
			r.StartedAt = &started
		}
		if scan.CompletedAt != nil {
			completed := scan.CompletedAt.Format("2006-01-02T15:04:05Z")
			r.CompletedAt = &completed
		}

		if scan.Results != "" {
			var results map[string]interface{}
			if err := json.Unmarshal([]byte(scan.Results), &results); err == nil {
				r.Results = results
			}
		}

		response[i] = r
	}

	c.JSON(http.StatusOK, response)
}

type LinkFoundGalleryRequest struct {
	Provider     string `json:"provider" binding:"required"`
	SourceURL    string `json:"source_url" binding:"required"`
	Name         string `json:"name" binding:"required"`
	ThumbnailURL string `json:"thumbnail_url"`
}

type LinkUnsureGalleryRequest struct {
	GalleryID uint   `json:"gallery_id" binding:"required"`
	Provider  string `json:"provider" binding:"required"`
	SourceURL string `json:"source_url" binding:"required"`
}

func LinkFoundGallery(c *gin.Context) {
	idStr := c.Param("id")
	personID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid person ID"})
		return
	}

	var req LinkFoundGalleryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var person models.Person
	if err := database.DB.First(&person, personID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	// Create gallery
	gallery := models.Gallery{
		Name:      req.Name,
		Provider:  req.Provider,
		SourceURL: req.SourceURL,
	}

	if err := database.DB.Create(&gallery).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create gallery"})
		return
	}

	// Download thumbnail if provided
	if req.ThumbnailURL != "" {
		localPath, err := services.DownloadProviderThumbnail(req.ThumbnailURL)
		if err != nil {
			logger.Warnf("Failed to download thumbnail for gallery %d: %v", gallery.ID, err)
		} else {
			gallery.ProviderThumbnail = localPath
			gallery.ProviderThumbnailURL = req.ThumbnailURL
			database.DB.Save(&gallery)
		}
	}

	// Link gallery to person
	database.DB.Model(&person).Association("Galleries").Append(&gallery)

	c.JSON(http.StatusCreated, gallery)
}

// LinkUnsureGallery links an existing gallery (unsure match) to a person
func LinkUnsureGallery(c *gin.Context) {
	idStr := c.Param("id")
	personID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid person ID"})
		return
	}

	var req LinkUnsureGalleryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var person models.Person
	if err := database.DB.First(&person, personID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	var gallery models.Gallery
	if err := database.DB.First(&gallery, req.GalleryID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gallery not found"})
		return
	}

	// Update gallery with provider info if not already set
	if gallery.Provider == "" {
		gallery.Provider = req.Provider
	}
	if gallery.SourceURL == "" {
		gallery.SourceURL = req.SourceURL
	}
	database.DB.Save(&gallery)

	// Link gallery to person
	database.DB.Model(&person).Association("Galleries").Append(&gallery)

	// Re-scan to update the cached results - this removes the linked gallery from unsure list
	go func() {
		// Get the provider alias for this provider
		var providerAlias models.PersonProviderAlias
		if err := database.DB.Where("person_id = ? AND provider = ?", personID, req.Provider).
			First(&providerAlias).Error; err == nil {
			if result, err := services.ScanSourceForPerson(uint(personID), req.Provider, providerAlias.Alias); err == nil {
				// Update the scan record with new results
				var latestScan models.PersonScanQueue
				if err := database.DB.Where("person_id = ? AND provider = ?", personID, req.Provider).
					Order("created_at DESC").First(&latestScan).Error; err == nil {
					resultsJSON, _ := json.Marshal(map[string]interface{}{
						"found_count":       result.FoundCount,
						"existing_count":    result.ExistingCount,
						"unsure_count":      result.UnsureCount,
						"missing_count":     result.MissingCount,
						"missing_galleries": result.MissingGalleries,
						"unsure_galleries":  result.UnsureGalleries,
					})
					latestScan.Results = string(resultsJSON)
					database.DB.Save(&latestScan)
				}
			}
		}
	}()

	c.JSON(http.StatusOK, gallery)
}

type ExcludeScanResultRequest struct {
	Provider  string `json:"provider" binding:"required"`
	SourceID  string `json:"source_id"`
	SourceURL string `json:"source_url"`
	Title     string `json:"title"`
	Reason    string `json:"reason"`
}

// ExcludeScanResult marks a scan result as not relevant to this person
// This prevents the same gallery from appearing in future scans
func ExcludeScanResult(c *gin.Context) {
	idStr := c.Param("id")
	personID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid person ID"})
		return
	}

	var req ExcludeScanResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify person exists
	var person models.Person
	if err := database.DB.First(&person, personID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	// Create exclusion record
	exclusion := models.ScanResultExclusion{
		PersonID:  uint(personID),
		Provider:  req.Provider,
		SourceID:  req.SourceID,
		SourceURL: req.SourceURL,
		Title:     req.Title,
		Reason:    req.Reason,
	}

	if err := database.DB.Create(&exclusion).Error; err != nil {
		logger.Errorf("Failed to create scan result exclusion: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to exclude scan result"})
		return
	}

	logger.Infof("Excluded scan result: person=%d, provider=%s, sourceID=%s", personID, req.Provider, req.SourceID)
	c.JSON(http.StatusOK, exclusion)
}

type AllMissingGalleriesResponse struct {
	PersonID     uint   `json:"person_id"`
	PersonName   string `json:"person_name"`
	Provider     string `json:"provider"`
	Alias        string `json:"alias"`
	GalleryURL   string `json:"gallery_url"`
	GalleryName  string `json:"gallery_name"`
	Thumbnail    string `json:"thumbnail"`
	FoundCount   int    `json:"found_count"`
	MissingCount int    `json:"missing_count"`
	ReleaseDate  string `json:"release_date"`
}

func GetAllMissingGalleries(c *gin.Context) {
	sortBy := c.DefaultQuery("sort", "name")

	var scans []models.PersonScanQueue
	if err := database.DB.Where("status = ?", models.ScanStatusCompleted).
		Order("created_at DESC").
		Find(&scans).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch scan results"})
		return
	}

	type personKey struct {
		personID uint
		provider string
	}
	latestScans := make(map[personKey]*models.PersonScanQueue)

	for i := range scans {
		key := personKey{personID: scans[i].PersonID, provider: scans[i].Provider}
		if _, exists := latestScans[key]; !exists {
			latestScans[key] = &scans[i]
		}
	}

	var response []AllMissingGalleriesResponse

	for _, scan := range latestScans {
		if scan.Results == "" {
			continue
		}

		var results map[string]interface{}
		if err := json.Unmarshal([]byte(scan.Results), &results); err != nil {
			continue
		}

		missingGalleries, ok := results["missing_galleries"].([]interface{})
		if !ok || len(missingGalleries) == 0 {
			continue
		}

		personName := "Unknown Person"
		var person models.Person
		if err := database.DB.First(&person, scan.PersonID).Error; err == nil {
			personName = person.Name
		}

		foundCount, _ := results["found_count"].(float64)
		missingCount, _ := results["missing_count"].(float64)

		for _, g := range missingGalleries {
			gMap, ok := g.(map[string]interface{})
			if !ok {
				continue
			}

			url, _ := gMap["url"].(string)
			title, _ := gMap["title"].(string)
			thumbnail, _ := gMap["thumbnail"].(string)
			releaseDate, _ := gMap["release_date"].(string)

			if url == "" {
				continue
			}

			if title == "" {
				title = "Untitled"
			}

			response = append(response, AllMissingGalleriesResponse{
				PersonID:     scan.PersonID,
				PersonName:   personName,
				Provider:     scan.Provider,
				Alias:        scan.Alias,
				GalleryURL:   url,
				GalleryName:  title,
				Thumbnail:    thumbnail,
				FoundCount:   int(foundCount),
				MissingCount: int(missingCount),
				ReleaseDate:  releaseDate,
			})
		}
	}

	switch sortBy {
	case "date":
		sort.Slice(response, func(i, j int) bool {
			if response[i].ReleaseDate == "" && response[j].ReleaseDate == "" {
				return response[i].PersonName < response[j].PersonName
			}
			if response[i].ReleaseDate == "" {
				return false
			}
			if response[j].ReleaseDate == "" {
				return true
			}
			return response[i].ReleaseDate > response[j].ReleaseDate
		})
	default:
		sort.Slice(response, func(i, j int) bool {
			return response[i].PersonName < response[j].PersonName
		})
	}

	c.JSON(http.StatusOK, response)
}
