package models

import (
	"time"

	"gorm.io/gorm"
)

// ImageEmbedding stores one normalized vector describing an image's content.
// The embedder column records which model/version produced the vector so that
// vectors from different model versions are never mixed in the same index.
type ImageEmbedding struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ImageID   uint      `gorm:"uniqueIndex;index" json:"image_id"`
	Embedder  string    `gorm:"index" json:"embedder"`
	Dimension int       `json:"dimension"`
	Vector    []byte    `json:"-" gorm:"type:BLOB"` // raw float32 little-endian bytes
	CreatedAt time.Time `json:"created_at"`
}

// ImageRating stores a user's 1-5 rating for an image. All records share
// user_id 0 until real users exist.
type ImageRating struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	ImageID   uint      `gorm:"index" json:"image_id"`
	Rating    int       `json:"rating"` // 1-5
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// EmbedQueue is a persisted, idempotent work queue for computing embeddings,
// tags and low-level features for images.
type EmbedQueue struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ImageID   uint      `gorm:"uniqueIndex" json:"image_id"`
	Status    string    `gorm:"index;default:'pending'" json:"status"` // pending | processing | failed | deferred
	Attempts  int       `json:"attempts"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (e *ImageEmbedding) BeforeSave(tx *gorm.DB) error {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	return nil
}