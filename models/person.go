package models

import (
	"time"

	"gorm.io/gorm"
)

type Person struct {
	ID           uint               `gorm:"primaryKey" json:"id"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
	DeletedAt    gorm.DeletedAt     `gorm:"index" json:"-"`
	Name         string             `gorm:"index" json:"name"`
	Aliases      string             `json:"aliases"`               // JSON array stored as text
	StashID      string             `gorm:"index" json:"stash_id"` // Deprecated: use Identifiers
	Birthdate    string             `json:"birthdate"`
	Country      string             `json:"country"`
	Ethnicity    string             `json:"ethnicity"`
	EyeColor     string             `json:"eye_color"`
	HairColor    string             `json:"hair_color"`
	Height       string             `json:"height"`
	Measurements string             `json:"measurements"`
	FakeTits     string             `json:"fake_tits"`
	CareerLength string             `json:"career_length"`
	Tattoos      string             `json:"tattoos"`
	Piercings    string             `json:"piercings"`
	Bio          string             `json:"bio"`
	Twitter      string             `json:"twitter"`
	Instagram    string             `json:"instagram"`
	Photos       string             `json:"photos"` // JSON array of image URLs
	Galleries    []*Gallery         `json:"galleries,omitempty" gorm:"many2many:person_galleries;"`
	Images       []*Image           `json:"images,omitempty" gorm:"many2many:person_images;"`
	Identifiers  []PersonIdentifier `json:"identifiers,omitempty" gorm:"foreignKey:PersonID"`
}

// PersonIdentifier stores external identifier information for a person
type PersonIdentifier struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	PersonID   uint      `gorm:"index" json:"person_id"`
	Source     string    `gorm:"index" json:"source"` // "stashdb", "tpdb", "iafd", etc.
	ExternalID string    `json:"external_id"`         // The ID from the external source
	Data       string    `json:"data"`                // JSON blob for source-specific data
}

// PersonExclusion tracks content that should NOT be tagged to a specific person
// Used to prevent auto-tagging false positives
type PersonExclusion struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	PersonID  uint      `gorm:"index" json:"person_id"`
	GalleryID *uint     `gorm:"index" json:"gallery_id,omitempty"` // Gallery to exclude (nullable)
	ImageID   *uint     `gorm:"index" json:"image_id,omitempty"`   // Image/video to exclude (nullable)
}
