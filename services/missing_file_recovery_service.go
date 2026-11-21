package services

import (
	"sync"

	"gallery_api/logger"
)

var (
	missingFileRecoveryMu      sync.Mutex
	missingFileRecoveryRunning bool
)

// StartMissingFileRecovery starts a coalesced missing-file recovery pass.
// The pass is asynchronous because restore/import requests must not wait for
// potentially long download work.
func StartMissingFileRecovery() bool {
	return startMissingFileRecovery(runMissingFileRecovery)
}

func startMissingFileRecovery(run func()) bool {
	missingFileRecoveryMu.Lock()
	if missingFileRecoveryRunning {
		missingFileRecoveryMu.Unlock()
		return false
	}
	missingFileRecoveryRunning = true
	missingFileRecoveryMu.Unlock()

	go func() {
		defer func() {
			missingFileRecoveryMu.Lock()
			missingFileRecoveryRunning = false
			missingFileRecoveryMu.Unlock()
		}()
		run()
	}()

	return true
}

// RunMissingFileRecovery synchronizes disk state and launches every existing
// missing-file verifier. It is synchronous for setup and launches the download
// work used by the image, video, and person-image verifiers in the background.
func RunMissingFileRecovery() {
	missingFileRecoveryMu.Lock()
	if missingFileRecoveryRunning {
		missingFileRecoveryMu.Unlock()
		return
	}
	missingFileRecoveryRunning = true
	missingFileRecoveryMu.Unlock()

	defer func() {
		missingFileRecoveryMu.Lock()
		missingFileRecoveryRunning = false
		missingFileRecoveryMu.Unlock()
	}()
	runMissingFileRecovery()
}

func runMissingFileRecovery() {
	logger.Info("Syncing file_exists column...")
	if err := SyncFileExists(); err != nil {
		logger.Error("File exists sync failed:", err)
	}

	logger.Info("Removing duplicate images...")
	if err := RemoveDuplicateImages(); err != nil {
		logger.Error("Duplicate image removal failed:", err)
	}

	logger.Info("Starting background verification of downloaded images...")
	if err := VerifyDownloadedImages(); err != nil {
		logger.Error("Background verification failed:", err)
	} else {
		logger.Info("Background verification started successfully")
	}

	logger.Info("Starting background verification of videos...")
	if err := VerifyDownloadedVideos(); err != nil {
		logger.Error("Video verification failed:", err)
	} else {
		logger.Info("Video verification started successfully")
	}

	logger.Info("Starting background verification of person images...")
	if err := VerifyPersonImages(); err != nil {
		logger.Error("Person image verification failed:", err)
	}
}
