package models

import (
	"time"

	"gorm.io/gorm"
)

type Image struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	CreatedAt       time.Time      `json:"created_at" gorm:"index"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
	GalleryID       uint           `gorm:"index" json:"gallery_id"` // Deprecated: use Galleries
	Gallery         *Gallery       `json:"gallery,omitempty" gorm:"foreignKey:GalleryID"`
	Galleries       []*Gallery     `json:"galleries,omitempty" gorm:"many2many:image_galleries;"`
	SourceID        *uint          `gorm:"index" json:"source_id,omitempty"` // Direct source association for videos
	Source          *Source        `json:"source,omitempty" gorm:"foreignKey:SourceID"`
	Filename        string         `json:"filename"`
	Title           string         `json:"title"`                     // Video title or display name
	Duration        float64        `json:"duration"`                  // Runtime in seconds
	Width           int            `json:"width"`                     // Video width in pixels
	Height          int            `json:"height"`                    // Video height in pixels
	SizeMB          float64        `json:"size_mb"`                   // File size in MB
	OriginalURL     string         `gorm:"index" json:"original_url"` // The hosting page URL (e.g., imagebam.com/view/...)
	DownloadURL     string         `json:"download_url" gorm:"index"` // The final direct image URL after ripping
	WebPath         string         `gorm:"-" json:"web_path"`
	ThumbnailPath   string         `gorm:"-" json:"thumbnail_path"`
	TrickplayVTT    string         `gorm:"-" json:"trickplay_vtt,omitempty"`       // VTT file path for video scrubbing
	TrickplaySprite string         `gorm:"-" json:"trickplay_sprite,omitempty"`    // Sprite sheet path for video scrubbing
	DominantColors  string         `json:"dominant_colors"`                        // JSON array of hex color strings
	IsFavorite      bool           `json:"is_favorite" gorm:"default:false;index"` // Favorite status
	Type            string         `json:"type" gorm:"default:'image';index"`      // "image" or "video"
	People          []*Person      `json:"people,omitempty" gorm:"many2many:person_images;"`
	Tags            []*Tag         `json:"tags,omitempty" gorm:"many2many:image_tags;"`
	VRMode          string         `json:"vr_mode" gorm:"default:'';index"` // "180" or "360" for persistent VR view
}
