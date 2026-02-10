package services

import (
	"gallery_api/database"
	"gallery_api/models"
	"sync"
	"sync/atomic"
)

type ProviderStatus struct {
	Active     int `json:"active"`
	MaxAllowed int `json:"max_allowed"`
}

type ActiveSourceInfo struct {
	ID               uint   `json:"id"`
	Name             string `json:"name"`
	Location         string `json:"location"`
	SourceName       string `json:"source_name,omitempty"`
	DownloadProgress int    `json:"download_progress"`
	DownloadedItems  int    `json:"downloaded_items"`
	TotalItems       int    `json:"total_items"`
}

type DownloadStatus struct {
	Crawler struct {
		ActiveSources []ActiveSourceInfo `json:"active_sources"`
	} `json:"crawler"`
	Verification struct {
		IsRunning       bool                      `json:"is_running"`
		TotalImages     int                       `json:"total_images"`
		Processed       int32                     `json:"processed"`
		MissingFound    int32                     `json:"missing_found"`
		Recovered       int32                     `json:"recovered"`
		ProviderStatus  map[string]ProviderStatus `json:"provider_status"`
		ActiveDownloads []ActiveSourceInfo        `json:"active_downloads"`
	} `json:"verification"`
	Videos struct {
		IsRunning       bool               `json:"is_running"`
		TotalVideos     int                `json:"total_videos"`
		Processed       int32              `json:"processed"`
		MissingFound    int32              `json:"missing_found"`
		Recovered       int32              `json:"recovered"`
		Active          int32              `json:"active"`
		MaxAllowed      int                `json:"max_allowed"`
		ActiveDownloads []ActiveSourceInfo `json:"active_downloads"`
	} `json:"videos"`
}

var (
	GlobalStatus DownloadStatus
	statusMutex  sync.RWMutex
)

func init() {
	GlobalStatus.Verification.ProviderStatus = make(map[string]ProviderStatus)
	GlobalStatus.Videos.MaxAllowed = 3 // From current hardcoded limit
}

func TriggerBroadcast() {
	BroadcastStatus(GetGlobalDownloadStatus())
}

func GetGlobalDownloadStatus() DownloadStatus {
	statusMutex.RLock()
	defer statusMutex.RUnlock()

	// Create a deep copy to avoid race conditions with maps/slices during JSON marshaling
	statusCopy := GlobalStatus

	// Deep copy the slices
	if GlobalStatus.Crawler.ActiveSources != nil {
		statusCopy.Crawler.ActiveSources = make([]ActiveSourceInfo, len(GlobalStatus.Crawler.ActiveSources))
		copy(statusCopy.Crawler.ActiveSources, GlobalStatus.Crawler.ActiveSources)
	}

	if GlobalStatus.Verification.ActiveDownloads != nil {
		statusCopy.Verification.ActiveDownloads = make([]ActiveSourceInfo, len(GlobalStatus.Verification.ActiveDownloads))
		copy(statusCopy.Verification.ActiveDownloads, GlobalStatus.Verification.ActiveDownloads)
	}

	if GlobalStatus.Videos.ActiveDownloads != nil {
		statusCopy.Videos.ActiveDownloads = make([]ActiveSourceInfo, len(GlobalStatus.Videos.ActiveDownloads))
		copy(statusCopy.Videos.ActiveDownloads, GlobalStatus.Videos.ActiveDownloads)
	}

	// Deep copy the map
	if GlobalStatus.Verification.ProviderStatus != nil {
		statusCopy.Verification.ProviderStatus = make(map[string]ProviderStatus)
		for k, v := range GlobalStatus.Verification.ProviderStatus {
			statusCopy.Verification.ProviderStatus[k] = v
		}
	}

	return statusCopy
}

// Verification Tracking
func SetVerificationRunning(running bool, total int) {
	statusMutex.Lock()
	GlobalStatus.Verification.IsRunning = running
	GlobalStatus.Verification.TotalImages = total
	GlobalStatus.Verification.Processed = 0
	GlobalStatus.Verification.MissingFound = 0
	GlobalStatus.Verification.Recovered = 0
	statusMutex.Unlock()
	TriggerBroadcast()
}

func IncVerificationProcessed() {
	atomic.AddInt32(&GlobalStatus.Verification.Processed, 1)
	// Don't broadcast every single image to avoid flood, maybe every 10 or so?
	// Actually for image verification it might be fast.
	// Let's just do it for now and see.
}

func IncVerificationMissing() {
	atomic.AddInt32(&GlobalStatus.Verification.MissingFound, 1)
}

func IncVerificationRecovered() {
	atomic.AddInt32(&GlobalStatus.Verification.Recovered, 1)
	TriggerBroadcast()
}

func UpdateProviderStatus(provider string, active int, max int) {
	statusMutex.Lock()
	GlobalStatus.Verification.ProviderStatus[provider] = ProviderStatus{
		Active:     active,
		MaxAllowed: max,
	}
	statusMutex.Unlock()
	TriggerBroadcast()
}

func AddActiveVerificationDownload(id uint, name string, location string, sourceName string) {
	statusMutex.Lock()
	GlobalStatus.Verification.ActiveDownloads = append(GlobalStatus.Verification.ActiveDownloads, ActiveSourceInfo{
		ID:         id,
		Name:       name,
		Location:   location,
		SourceName: sourceName,
	})
	statusMutex.Unlock()
	TriggerBroadcast()
}

func RemoveActiveVerificationDownload(id uint) {
	statusMutex.Lock()
	var newActive []ActiveSourceInfo
	for _, info := range GlobalStatus.Verification.ActiveDownloads {
		if info.ID != id {
			newActive = append(newActive, info)
		}
	}
	GlobalStatus.Verification.ActiveDownloads = newActive
	statusMutex.Unlock()
	TriggerBroadcast()
}

// Video Verification Tracking
func SetVideoVerificationRunning(running bool, total int) {
	statusMutex.Lock()
	GlobalStatus.Videos.IsRunning = running
	GlobalStatus.Videos.TotalVideos = total
	GlobalStatus.Videos.Processed = 0
	GlobalStatus.Videos.MissingFound = 0
	GlobalStatus.Videos.Recovered = 0
	statusMutex.Unlock()
	TriggerBroadcast()
}

func IncVideoVerificationProcessed() {
	atomic.AddInt32(&GlobalStatus.Videos.Processed, 1)
}

func IncVideoVerificationMissing() {
	atomic.AddInt32(&GlobalStatus.Videos.MissingFound, 1)
}

func IncVideoVerificationRecovered() {
	atomic.AddInt32(&GlobalStatus.Videos.Recovered, 1)
	TriggerBroadcast()
}

func UpdateVideoActiveCount(delta int32) {
	atomic.AddInt32(&GlobalStatus.Videos.Active, delta)
	TriggerBroadcast()
}

func AddActiveVideoDownload(id uint, name string, location string, sourceName string) {
	statusMutex.Lock()
	GlobalStatus.Videos.ActiveDownloads = append(GlobalStatus.Videos.ActiveDownloads, ActiveSourceInfo{
		ID:         id,
		Name:       name,
		Location:   location,
		SourceName: sourceName,
	})
	statusMutex.Unlock()
	TriggerBroadcast()
}

func RemoveActiveVideoDownload(id uint) {
	statusMutex.Lock()
	var newActive []ActiveSourceInfo
	for _, info := range GlobalStatus.Videos.ActiveDownloads {
		if info.ID != id {
			newActive = append(newActive, info)
		}
	}
	GlobalStatus.Videos.ActiveDownloads = newActive
	statusMutex.Unlock()
	TriggerBroadcast()
}

// Crawler Tracking
func AddActiveCrawlerSource(id uint) {
	var source models.Source
	if err := database.DB.First(&source, id).Error; err != nil {
		return
	}

	statusMutex.Lock()
	GlobalStatus.Crawler.ActiveSources = append(GlobalStatus.Crawler.ActiveSources, ActiveSourceInfo{
		ID:               source.ID,
		Name:             source.Name,
		Location:         source.Location,
		DownloadProgress: source.DownloadProgress,
		DownloadedItems:  source.DownloadedItems,
		TotalItems:       source.TotalItems,
	})
	statusMutex.Unlock()
	TriggerBroadcast()
}

func UpdateCrawlerProgress(id uint, progress int, downloaded int, total int) {
	statusMutex.Lock()
	for i := range GlobalStatus.Crawler.ActiveSources {
		if GlobalStatus.Crawler.ActiveSources[i].ID == id {
			GlobalStatus.Crawler.ActiveSources[i].DownloadProgress = progress
			GlobalStatus.Crawler.ActiveSources[i].DownloadedItems = downloaded
			GlobalStatus.Crawler.ActiveSources[i].TotalItems = total
			break
		}
	}
	statusMutex.Unlock()
	TriggerBroadcast()
}

func RemoveActiveCrawlerSource(id uint) {
	statusMutex.Lock()
	var newActive []ActiveSourceInfo
	for _, info := range GlobalStatus.Crawler.ActiveSources {
		if info.ID != id {
			newActive = append(newActive, info)
		}
	}
	GlobalStatus.Crawler.ActiveSources = newActive
	statusMutex.Unlock()
	TriggerBroadcast()
}
