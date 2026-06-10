package models

import (
	"time"

	"gorm.io/gorm"
)

type Gallery struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at" gorm:"index"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Name      string         `gorm:"index" json:"name"`
	SourceID  *uint          `gorm:"index" json:"source_id"` // Nullable if created manually
	Source    *Source        `json:"source,omitempty" gorm:"foreignKey:SourceID"`
	Images    []Image        `json:"images,omitempty" gorm:"foreignKey:GalleryID"`
	People    []*Person      `json:"people,omitempty" gorm:"many2many:person_galleries;"`

	// New metadata fields for gallery scraping
	Description          string    `json:"description"`
	Provider             string    `json:"provider"` // e.g., "Playboy", "Metart"
	SourceURL            string    `json:"source_url"`
	ProviderThumbnail    string    `json:"provider_thumbnail"`          // Local path to downloaded thumbnail
	ProviderThumbnailURL string    `json:"provider_thumbnail_url"`      // Original URL for re-download
	CoverImageID         *uint     `json:"cover_image_id" gorm:"index"` // Optional manually selected cover image
	Rating               float64   `json:"rating"`
	ReleaseDate          time.Time `json:"release_date"`
}
