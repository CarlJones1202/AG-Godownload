package models

import (
	"time"

	"gorm.io/gorm"
)

type Source struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Name      string         `json:"name"`
	Type      string         `json:"type"` // e.g., "url", "local"
	Location  string         `json:"location"`
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
	OriginalURL string         `json:"original_url"`
}
