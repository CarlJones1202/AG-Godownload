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
	Type          string         `json:"type"` // e.g., "url", "local"
	Location      string         `json:"location"`
	LastCheckedAt time.Time      `json:"last_checked_at"`
	Status        string         `json:"status"` // "idle", "crawling", "error"
}

type Gallery struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Name      string         `json:"name"`
	SourceID  *uint          `json:"source_id"` // Nullable if created manually
	Images    []Image        `json:"images,omitempty"`
}

type Image struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	GalleryID   uint           `json:"gallery_id"`
	Filename    string         `json:"filename"`
	OriginalURL string         `json:"original_url"` // The hosting page URL (e.g., imagebam.com/view/...)
	DownloadURL string         `json:"download_url"` // The final direct image URL after ripping
}
