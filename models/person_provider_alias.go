package models

import (
	"time"

	"gorm.io/gorm"
)

type PersonProviderAlias struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	PersonID  uint           `gorm:"index" json:"person_id"`
	Provider  string         `gorm:"index" json:"provider"`
	Alias     string         `gorm:"index" json:"alias"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
