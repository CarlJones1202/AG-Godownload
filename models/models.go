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
	Location      string         `gorm:"uniqueIndex" json:"location"`
	LastCheckedAt time.Time      `gorm:"index" json:"last_checked_at"`
	Status        string         `gorm:"index" json:"status"` // "idle", "crawling", "error"
}

type Gallery struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Name      string         `json:"name"`
	SourceID  *uint          `gorm:"index" json:"source_id"` // Nullable if created manually
	Images    []Image        `json:"images,omitempty" gorm:"foreignKey:GalleryID"`
	People    []*Person      `json:"people,omitempty" gorm:"many2many:person_galleries;"`
}

type Image struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	GalleryID   uint           `gorm:"index" json:"gallery_id"` // Deprecated: use Galleries
	Gallery     *Gallery       `json:"gallery,omitempty" gorm:"foreignKey:GalleryID"`
	Galleries   []*Gallery     `json:"galleries,omitempty" gorm:"many2many:image_galleries;"`
	Filename    string         `json:"filename"`
	OriginalURL string         `gorm:"index" json:"original_url"` // The hosting page URL (e.g., imagebam.com/view/...)
	DownloadURL string         `json:"download_url"`              // The final direct image URL after ripping
}

type Person struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Name      string         `gorm:"index" json:"name"`
	Aliases   string         `json:"aliases"` // JSON array stored as text
	StashID   string         `gorm:"index" json:"stash_id"`
	Galleries []*Gallery     `json:"galleries,omitempty" gorm:"many2many:person_galleries;"`
}
