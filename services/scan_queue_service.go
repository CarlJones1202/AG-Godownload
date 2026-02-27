package services

import (
	"encoding/json"
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

type ScanQueue struct {
	mu            sync.Mutex
	queue         []uint // IDs of PersonScanQueue
	cond          *sync.Cond
	running       bool
	providerLocks map[string]*sync.Mutex
}

var scanQueue *ScanQueue = &ScanQueue{
	providerLocks: make(map[string]*sync.Mutex),
}

func init() {
	scanQueue.cond = sync.NewCond(&scanQueue.mu)
}

// AddToScanQueue adds a person scan job to the queue
func AddToScanQueue(personID uint, provider string, alias string) error {
	// Check if a scan for this (person_id, provider) already exists
	var existingScan models.PersonScanQueue
	result := database.DB.Where("person_id = ? AND provider = ?", personID, provider).
		Order("created_at DESC").
		First(&existingScan)

	if result.Error == nil {
		// Scan exists - check its status
		if existingScan.Status == models.ScanStatusProcessing {
			// A scan is currently processing, skip to avoid duplicates
			logger.Infof("Scan already processing for person=%d, provider=%s, skipping duplicate", personID, provider)
			return nil
		} else if existingScan.Status == models.ScanStatusCompleted || existingScan.Status == models.ScanStatusFailed {
			// Delete the old scan and create a new one
			if err := database.DB.Delete(&existingScan).Error; err != nil {
				logger.Warnf("Failed to delete old scan %d: %v", existingScan.ID, err)
			} else {
				logger.Infof("Deleted old scan %d for person=%d, provider=%s", existingScan.ID, personID, provider)
			}
		}
	}

	// Create new scan
	scan := models.PersonScanQueue{
		PersonID: personID,
		Provider: provider,
		Alias:    alias,
		Status:   models.ScanStatusPending,
	}

	if err := database.DB.Create(&scan).Error; err != nil {
		return fmt.Errorf("failed to create scan queue entry: %w", err)
	}

	scanQueue.mu.Lock()
	scanQueue.queue = append(scanQueue.queue, scan.ID)
	scanQueue.cond.Signal()
	scanQueue.mu.Unlock()

	logger.Infof("Added scan job to queue: person=%d, provider=%s, alias=%s", personID, provider, alias)
	return nil
}

// AddAllPeopleToScanQueue adds all people with provider aliases to the scan queue
func AddAllPeopleToScanQueue() error {
	var aliases []models.PersonProviderAlias
	if err := database.DB.Find(&aliases).Error; err != nil {
		return fmt.Errorf("failed to fetch provider aliases: %w", err)
	}

	seen := make(map[string]bool) // Track (personID, provider) pairs

	for _, alias := range aliases {
		key := fmt.Sprintf("%d-%s", alias.PersonID, alias.Provider)
		if seen[key] {
			continue
		}
		seen[key] = true

		if err := AddToScanQueue(alias.PersonID, alias.Provider, alias.Alias); err != nil {
			logger.Warnf("Failed to add scan for person %d: %v", alias.PersonID, err)
		}
	}

	logger.Infof("Added %d people to scan queue", len(seen))
	return nil
}

// GetScanResults returns the scan results for a person
func GetScanResults(personID uint) ([]models.PersonScanQueue, error) {
	var scans []models.PersonScanQueue
	if err := database.DB.Where("person_id = ?", personID).
		Order("created_at DESC").
		Find(&scans).Error; err != nil {
		return nil, err
	}
	return scans, nil
}

// GetLatestScanResult returns the most recent scan result for a person and provider
func GetLatestScanResult(personID uint, provider string) (*models.PersonScanQueue, error) {
	var scan models.PersonScanQueue
	if err := database.DB.Where("person_id = ? AND provider = ?", personID, provider).
		Order("created_at DESC").
		First(&scan).Error; err != nil {
		return nil, err
	}
	return &scan, nil
}

// StartScanWorker starts the background worker that processes scan jobs
func StartScanWorker() {
	go func() {
		logger.Info("Starting scan worker")
		for {
			scanQueue.mu.Lock()
			for len(scanQueue.queue) == 0 {
				scanQueue.cond.Wait()
			}
			scanID := scanQueue.queue[0]
			scanQueue.queue = scanQueue.queue[1:]
			scanQueue.mu.Unlock()

			processScan(scanID)

			// Small pause between processing items
			time.Sleep(500 * time.Millisecond)
		}
	}()
}

func processScan(scanID uint) {
	var scan models.PersonScanQueue
	if err := database.DB.First(&scan, scanID).Error; err != nil {
		logger.Errorf("Failed to find scan %d: %v", scanID, err)
		return
	}

	// Check if already processed
	if scan.Status == models.ScanStatusCompleted || scan.Status == models.ScanStatusFailed {
		return
	}

	// Get or create provider lock
	provider := scan.Provider
	scanQueue.mu.Lock()
	if _, ok := scanQueue.providerLocks[provider]; !ok {
		scanQueue.providerLocks[provider] = &sync.Mutex{}
	}
	providerLock := scanQueue.providerLocks[provider]
	scanQueue.mu.Unlock()

	// Lock this provider
	providerLock.Lock()

	// Update status to processing
	now := time.Now()
	scan.Status = models.ScanStatusProcessing
	scan.StartedAt = &now
	database.DB.Save(&scan)

	logger.Infof("Processing scan %d: person=%d, provider=%s", scanID, scan.PersonID, scan.Provider)

	// Execute the scan using existing function
	result, err := ScanSourceForPerson(scan.PersonID, scan.Provider, scan.Alias)
	if err != nil {
		scan.Status = models.ScanStatusFailed
		scan.Error = err.Error()
		logger.Errorf("Scan %d failed: %v", scanID, err)
	} else {
		// Serialize results
		resultsJSON, err := json.Marshal(result)
		if err != nil {
			scan.Status = models.ScanStatusFailed
			scan.Error = fmt.Sprintf("failed to serialize results: %v", err)
		} else {
			scan.Status = models.ScanStatusCompleted
			scan.Results = string(resultsJSON)
			logger.Infof("Scan %d completed: found=%d, missing=%d", scanID, result.FoundCount, result.MissingCount)
		}
	}

	completedAt := time.Now()
	scan.CompletedAt = &completedAt
	database.DB.Save(&scan)

	providerLock.Unlock()
}

// StartDailyScanScheduler starts a daily cron job that scans all people with provider aliases
func StartDailyScanScheduler() {
	c := cron.New()
	// Run daily at 3 AM
	_, err := c.AddFunc("0 3 * * *", func() {
		logger.Info("Starting daily scan of all people with provider aliases")
		if err := AddAllPeopleToScanQueue(); err != nil {
			logger.Errorf("Daily scan failed: %v", err)
		}
	})
	if err != nil {
		logger.Errorf("Failed to setup daily scan scheduler: %v", err)
		return
	}
	c.Start()
	logger.Info("Daily scan scheduler started (runs at 3 AM)")
}

// CleanupDuplicateScans removes duplicate scan results for the same person+provider combination
// Keeps only the most recent completed scan and removes any pending/processing ones
func CleanupDuplicateScans() error {
	logger.Info("Cleaning up duplicate scan results...")

	var allScans []models.PersonScanQueue
	if err := database.DB.Find(&allScans).Error; err != nil {
		return fmt.Errorf("failed to fetch scans: %w", err)
	}

	// Group scans by (person_id, provider)
	type scanKey struct {
		personID uint
		provider string
	}
	scansByKey := make(map[scanKey][]models.PersonScanQueue)

	for _, scan := range allScans {
		key := scanKey{personID: scan.PersonID, provider: scan.Provider}
		scansByKey[key] = append(scansByKey[key], scan)
	}

	deletedCount := 0

	// For each (person_id, provider) pair, keep only the most recent scan
	for _, scans := range scansByKey {
		if len(scans) <= 1 {
			continue // No duplicates for this pair
		}

		// Sort by created_at descending to find the most recent
		// Note: GORM returns results sorted by default, but let's be explicit
		var mostRecent *models.PersonScanQueue
		for i := range scans {
			if mostRecent == nil || scans[i].CreatedAt.After(mostRecent.CreatedAt) {
				mostRecent = &scans[i]
			}
		}

		// Delete all other scans
		for i := range scans {
			if scans[i].ID != mostRecent.ID {
				if err := database.DB.Delete(&scans[i]).Error; err != nil {
					logger.Warnf("Failed to delete duplicate scan %d: %v", scans[i].ID, err)
				} else {
					deletedCount++
					logger.Debugf("Deleted duplicate scan %d (person=%d, provider=%s, kept scan %d)",
						scans[i].ID, scans[i].PersonID, scans[i].Provider, mostRecent.ID)
				}
			}
		}
	}

	logger.Infof("Cleanup complete: deleted %d duplicate scans", deletedCount)
	return nil
}
