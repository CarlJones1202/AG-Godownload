package handlers

import (
	"encoding/json"
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"sort"
	"strconv"
	"strings"

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

	// Auto-resolve any missing galleries that match this gallery by name
	if err := services.AutoResolveMissingGalleries(person.ID, gallery.ID); err != nil {
		logger.Warnf("Failed to auto-resolve missing galleries for person %d, gallery %d: %v", person.ID, gallery.ID, err)
	}

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

	// Auto-resolve any missing galleries that match this gallery by name
	if err := services.AutoResolveMissingGalleries(person.ID, gallery.ID); err != nil {
		logger.Warnf("Failed to auto-resolve missing galleries for person %d, gallery %d: %v", person.ID, gallery.ID, err)
	}

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
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}

	sortBy := c.DefaultQuery("sort", "name")
	providerFilter := strings.ToLower(c.Query("provider"))
	q := strings.ToLower(c.Query("q"))
	var personIDFilter uint
	if pid := c.Query("person_id"); pid != "" {
		if parsed, err := strconv.ParseUint(pid, 10, 32); err == nil {
			personIDFilter = uint(parsed)
		}
	}

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

	// Preload person names in one query to avoid N+1 lookups
	personNames := make(map[uint]string)
	{
		var people []models.Person
		if err := database.DB.Find(&people).Error; err != nil {
			logger.Warnf("Failed to load people for missing galleries: %v", err)
		} else {
			for _, p := range people {
				personNames[p.ID] = p.Name
			}
		}
	}

	var response []AllMissingGalleriesResponse

	// Load exclusions for this person (or all people) up front so hidden
	// ("not wanted") galleries stay filtered out of the list.
	var exclusions []models.ScanResultExclusion
	if personIDFilter != 0 {
		database.DB.Where("person_id = ?", personIDFilter).Find(&exclusions)
	} else {
		database.DB.Find(&exclusions)
	}
	excludedByKey := make(map[string]bool)
	for _, ex := range exclusions {
		if ex.SourceURL != "" {
			excludedByKey[fmt.Sprintf("%d|%s|%s", ex.PersonID, strings.ToLower(ex.Provider), ex.SourceURL)] = true
		}
	}

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

		if personIDFilter != 0 && scan.PersonID != personIDFilter {
			continue
		}
		if providerFilter != "" && strings.ToLower(scan.Provider) != providerFilter {
			continue
		}

		personName := personNames[scan.PersonID]
		if personName == "" {
			personName = "Unknown Person"
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

			if excludedByKey[fmt.Sprintf("%d|%s|%s", scan.PersonID, strings.ToLower(scan.Provider), url)] {
				continue
			}

			if title == "" {
				title = "Untitled"
			}

			if q != "" && !strings.Contains(strings.ToLower(title), q) && !strings.Contains(strings.ToLower(personName), q) {
				continue
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
		sort.SliceStable(response, func(i, j int) bool {
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
		sort.SliceStable(response, func(i, j int) bool {
			if response[i].PersonName == response[j].PersonName {
				return response[i].GalleryName < response[j].GalleryName
			}
			return response[i].PersonName < response[j].PersonName
		})
	}

	total := len(response)

	// Paginate
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	paged := response[start:end]

	// Count in-flight scans so the UI can show live recheck progress
	var pendingScans int64
	database.DB.Model(&models.PersonScanQueue{}).
		Where("status IN ?", []models.ScanStatus{models.ScanStatusPending, models.ScanStatusProcessing}).
		Count(&pendingScans)

	totalPages := 0
	if limit > 0 {
		totalPages = (total + limit - 1) / limit
	}

	c.JSON(http.StatusOK, gin.H{
		"data": paged,
		"meta": gin.H{
			"current_page":  page,
			"total_pages":   totalPages,
			"total_items":   total,
			"limit":         limit,
			"pending_scans": pendingScans,
		},
	})
}

func RecheckAllPeople(c *gin.Context) {
	queued, err := services.AddAllPeopleToScanQueue()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Recheck triggered for all people across all alias+provider combos",
		"queued":  queued,
	})
}

// NotWantedExclusionResponse is an enriched scan-result exclusion (a missing
// gallery marked "not wanted" for a given person+provider).
type NotWantedExclusionResponse struct {
	ID         uint   `json:"id"`
	PersonID   uint   `json:"person_id"`
	PersonName string `json:"person_name"`
	Provider   string `json:"provider"`
	SourceURL  string `json:"source_url"`
	Title      string `json:"title"`
	Reason     string `json:"reason"`
	CreatedAt  string `json:"created_at"`
}

// GetNotWantedGalleries lists all missing galleries marked "not wanted"
// (i.e. scan-result exclusions), optionally scoped to a person.
func GetNotWantedGalleries(c *gin.Context) {
	var exclusions []models.ScanResultExclusion
	query := database.DB.Model(&models.ScanResultExclusion{})
	if pid := c.Query("person_id"); pid != "" {
		query = query.Where("person_id = ?", pid)
	}
	if err := query.Order("created_at DESC").Find(&exclusions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch not-wanted galleries"})
		return
	}

	personNames := make(map[uint]string)
	var people []models.Person
	if err := database.DB.Find(&people).Error; err == nil {
		for _, p := range people {
			personNames[p.ID] = p.Name
		}
	}

	response := make([]NotWantedExclusionResponse, 0, len(exclusions))
	for _, ex := range exclusions {
		name := personNames[ex.PersonID]
		if name == "" {
			name = "Unknown Person"
		}
		response = append(response, NotWantedExclusionResponse{
			ID:         ex.ID,
			PersonID:   ex.PersonID,
			PersonName: name,
			Provider:   ex.Provider,
			SourceURL:  ex.SourceURL,
			Title:      ex.Title,
			Reason:     ex.Reason,
			CreatedAt:  ex.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": response})
}

// RemoveNotWantedGallery unmarks a gallery, allowing it to reappear in the
// missing-galleries list.
func RemoveNotWantedGallery(c *gin.Context) {
	idStr := c.Param("id")
	exclusionID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid exclusion ID"})
		return
	}

	var exclusion models.ScanResultExclusion
	if err := database.DB.First(&exclusion, exclusionID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Not-wanted entry not found"})
		return
	}

	if err := database.DB.Delete(&exclusion).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove not-wanted entry"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Gallery unmarked"})
}

