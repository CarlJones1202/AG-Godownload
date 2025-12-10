package models

import (
	"time"

	"gorm.io/gorm"
)

type Tag struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Name      string         `gorm:"uniqueIndex" json:"name"`
	Category  string         `gorm:"index" json:"category"` // "label", "pose", "mood", "manual"
	Images    []*Image       `json:"images,omitempty" gorm:"many2many:image_tags;"`
}
