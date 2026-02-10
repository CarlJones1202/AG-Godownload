package models

import (
	"time"

	"gorm.io/gorm"
)

type Source struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
	Name          string         `json:"name"`
	Type          string         `json:"type" gorm:"index"` // e.g., "url", "local"
	Location      string         `gorm:"uniqueIndex" json:"location"`
	LastCheckedAt time.Time      `gorm:"index" json:"last_checked_at"`
	Status        string         `gorm:"index" json:"status"` // "idle", "crawling", "error"
}
