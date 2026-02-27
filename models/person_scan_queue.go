package models

import (
	"time"

	"gorm.io/gorm"
)

type ScanStatus string

const (
	ScanStatusPending    ScanStatus = "pending"
	ScanStatusProcessing ScanStatus = "processing"
	ScanStatusCompleted  ScanStatus = "completed"
	ScanStatusFailed     ScanStatus = "failed"
)

type PersonScanQueue struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	PersonID    uint           `gorm:"index" json:"person_id"`
	Provider    string         `gorm:"index" json:"provider"`
	Alias       string         `json:"alias"`
	Status      ScanStatus     `gorm:"index" json:"status"`
	Results     string         `json:"results"`         // JSON string of scan results
	Error       string         `json:"error,omitempty"` // Error message if failed
	CreatedAt   time.Time      `json:"created_at"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}
