package models

import (
	"time"

	"gorm.io/gorm"
)

// ScanResultExclusion marks a scan result as not relevant to a person
// This prevents the same gallery from appearing in future scans for this person+provider
type ScanResultExclusion struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	PersonID  uint           `gorm:"index" json:"person_id"`
	Provider  string         `gorm:"index" json:"provider"`
	SourceID  string         `gorm:"index" json:"source_id"` // Unique ID from provider (MetArt UUID, etc)
	SourceURL string         `json:"source_url"`             // URL of the gallery for reference
	Title     string         `json:"title"`                  // Gallery title for reference
	Reason    string         `json:"reason,omitempty"`       // Optional reason for exclusion
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
