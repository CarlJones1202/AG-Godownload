package services

import (
	"gallery_api/logger"
)

// AITagResult represents the JSON output from the Python script
type AITagResult struct {
	Content []AITagItem `json:"content"`
	Pose    []AITagItem `json:"pose"`
	Mood    []AITagItem `json:"mood"`
	Vibe    []AITagItem `json:"vibe"`
	Error   string      `json:"error,omitempty"`
}

type AITagItem struct {
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
}

// LabelImage is intentionally disabled/commented out. The AI tagging
// feature was removed as requested. This stub remains so other packages
// that reference LabelImage will continue to link. It does nothing.
func LabelImage(imageID uint) error {
	logger.Infof("LabelImage called but AI tagging is disabled; image=%d", imageID)
	return nil
}

// ScanUntaggedImages used to enqueue images for AI tagging. It's retained as
// a no-op stub to avoid breaking callers.
func ScanUntaggedImages() error {
	logger.Info("ScanUntaggedImages called but AI tagging is disabled; no action taken")
	return nil
}
