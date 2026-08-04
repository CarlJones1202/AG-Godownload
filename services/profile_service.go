package services

import (
	"errors"
	"fmt"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"sync"

	"gorm.io/gorm"
)

const (
	// defaultUserID is the shared profile until real users exist.
	defaultUserID uint = 0
	// Like/Dislike rating thresholds. 3 = neutral, ignored for the profile.
	likeThreshold    = 4
	dislikeThreshold = 2
	// dislikeWeight dampens disliked images' pull in the Rocchio vector.
	dislikeWeight = 0.6
)

// TasteProfile is the user's learned preference model. It maintains running
// (sum, count) statistics in embedding space plus per-tag like/dislike counts,
// so a rating event is an O(1) update.
type TasteProfile struct {
	mu          sync.RWMutex
	embedder    string
	likeSum     []float32
	dislikeSum  []float32
	nLikes      int
	nDislikes   int
	tagLike     map[uint]int
	tagDislike  map[uint]int
}

func NewTasteProfile() *TasteProfile {
	return &TasteProfile{
		embedder:   CurrentEmbedder().Name(),
		tagLike:    make(map[uint]int),
		tagDislike: make(map[uint]int),
	}
}

// DefaultProfile is the shared (user 0) taste profile.
var DefaultProfile = NewTasteProfile()

// bootstrappedProfile tracks whether Rebuild has been run at startup.
var bootstrappedProfile bool

// Rebuild reloads the profile from every stored rating. Call once at startup.
func (p *TasteProfile) Rebuild() error {
	var ratings []models.ImageRating
	if err := database.DB.Where("user_id = ?", defaultUserID).Find(&ratings).Error; err != nil {
		return err
	}
	p.Reset()
	for _, r := range ratings {
		p.Add(r.ImageID, r.Rating)
	}
	bootstrappedProfile = true
	logger.Infof("Taste profile rebuilt from %d ratings (%d likes, %d dislikes)", len(ratings), p.nLikes, p.nDislikes)
	return nil
}

// Reset zeroes all accumulated preference state.
func (p *TasteProfile) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.likeSum = nil
	p.dislikeSum = nil
	p.nLikes = 0
	p.nDislikes = 0
	p.tagLike = make(map[uint]int)
	p.tagDislike = make(map[uint]int)
}

// Add folds one rating into the profile (positive if rating >= likeThreshold,
// negative if rating <= dislikeThreshold, ignored otherwise). Missing vectors
// are skipped silently — the profile catches up on the next Rebuild.
func (p *TasteProfile) Add(imageID uint, rating int) {
	like := rating >= likeThreshold
	dislike := rating <= dislikeThreshold
	if !like && !dislike {
		return
	}

	vec, err := LoadVector(imageID)
	if err != nil {
		logger.Debugf("Profile: no vector for image %d, skipping contribution", imageID)
	}
	tags := imageTagIDs(imageID)

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.embedder != CurrentEmbedder().Name() {
		p.likeSum = nil
		p.dislikeSum = nil
		p.embedder = CurrentEmbedder().Name()
	}
	if vec != nil {
		if like {
			if p.likeSum == nil {
				p.likeSum = make([]float32, len(vec))
			}
			for i, f := range vec {
				p.likeSum[i] += f
			}
			p.nLikes++
		} else if dislike {
			if p.dislikeSum == nil {
				p.dislikeSum = make([]float32, len(vec))
			}
			for i, f := range vec {
				p.dislikeSum[i] += f
			}
			p.nDislikes++
		}
	}
	for _, tid := range tags {
		if like {
			p.tagLike[tid]++
		} else if dislike {
			p.tagDislike[tid]++
		}
	}
}

// Remove folds an old rating out of the profile (used when a rating changes or
// is deleted).
func (p *TasteProfile) Remove(imageID uint, oldRating int) {
	like := oldRating >= likeThreshold
	dislike := oldRating <= dislikeThreshold
	if !like && !dislike {
		return
	}

	vec, err := LoadVector(imageID)
	if err != nil {
		logger.Debugf("Profile: no vector for image %d while removing, skipping", imageID)
	}
	tags := imageTagIDs(imageID)

	p.mu.Lock()
	defer p.mu.Unlock()
	if vec != nil {
		if like && p.nLikes > 0 {
			for i, f := range vec {
				p.likeSum[i] -= f
			}
			p.nLikes--
		} else if dislike && p.nDislikes > 0 {
			for i, f := range vec {
				p.dislikeSum[i] -= f
			}
			p.nDislikes--
		}
	}
	for _, tid := range tags {
		if like {
			p.tagLike[tid]--
			if p.tagLike[tid] <= 0 {
				delete(p.tagLike, tid)
			}
		} else if dislike {
			p.tagDislike[tid]--
			if p.tagDislike[tid] <= 0 {
				delete(p.tagDislike, tid)
			}
		}
	}
}

// SetRating stores a rating in the database and updates the in-memory profile
// in one call. Returns nil on success.
func (p *TasteProfile) SetRating(imageID uint, rating int) error {
	if rating < 1 || rating > 5 {
		return fmt.Errorf("rating out of range 1-5")
	}

	var existing models.ImageRating
	err := database.DB.Where("user_id = ? AND image_id = ?", defaultUserID, imageID).First(&existing).Error
	if err == nil {
		if existing.Rating != rating {
			p.Remove(imageID, existing.Rating)
			existing.Rating = rating
			if err := database.DB.Save(&existing).Error; err != nil {
				return err
			}
			p.Add(imageID, rating)
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	row := models.ImageRating{UserID: defaultUserID, ImageID: imageID, Rating: rating}
	if err := database.DB.Create(&row).Error; err != nil {
		return err
	}
	p.Add(imageID, rating)
	return nil
}

// GetRating returns the stored rating for an image (0 when none).
func (p *TasteProfile) GetRating(imageID uint) (int, error) {
	var row models.ImageRating
	err := database.DB.Where("user_id = ? AND image_id = ?", defaultUserID, imageID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return row.Rating, nil
}

// ClearRating deletes a stored rating and removes its profile contribution.
func (p *TasteProfile) ClearRating(imageID uint) error {
	var row models.ImageRating
	err := database.DB.Where("user_id = ? AND image_id = ?", defaultUserID, imageID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	p.Remove(imageID, row.Rating)
	return database.DB.Delete(&row).Error
}

// ResetAll wipes the shared profile and all stored ratings.
func (p *TasteProfile) ResetAll() error {
	p.Reset()
	return database.DB.Where("user_id = ?", defaultUserID).Delete(&models.ImageRating{}).Error
}

// Ready reports whether the profile has enough signal to personalize.
func (p *TasteProfile) Ready() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.nLikes >= 3 && p.nDislikes >= 1
}

// Counts returns the current (likes, dislikes) counts.
func (p *TasteProfile) Counts() (int, int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.nLikes, p.nDislikes
}

// Vector returns the normalized preference vector (like centroid - dislike
// centroid), or nil when there is nothing to go on.
func (p *TasteProfile) Vector() []float32 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.nLikes == 0 {
		return nil
	}
	dim := 0
	if p.likeSum != nil {
		dim = len(p.likeSum)
	} else if p.dislikeSum != nil {
		dim = len(p.dislikeSum)
	} else {
		return nil
	}
	v := make([]float32, dim)
	if p.nLikes > 0 && p.likeSum != nil {
		for i, f := range p.likeSum {
			v[i] += float32(float64(f) / float64(p.nLikes))
		}
	}
	if p.nDislikes > 0 && p.dislikeSum != nil {
		for i, f := range p.dislikeSum {
			v[i] -= float32(dislikeWeight * float64(f) / float64(p.nDislikes))
		}
	}
	return normalizeVec(v)
}

// TagAffinity returns a smoothed positive/negative affinity per tag id.
// Affinity is in (-0.6, 1]; positive means the user tends to like images with
// this tag, negative the opposite.
func (p *TasteProfile) TagAffinity() map[uint]float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make(map[uint]float64, len(p.tagLike)+len(p.tagDislike))
	ids := make(map[uint]bool, len(p.tagLike)+len(p.tagDislike))
	for id := range p.tagLike {
		ids[id] = true
	}
	for id := range p.tagDislike {
		ids[id] = true
	}
	for id := range ids {
		likes := p.tagLike[id]
		dislikes := p.tagDislike[id]
		out[id] = (float64(likes) - dislikeWeight*float64(dislikes)) / (float64(likes+dislikes) + 2)
	}
	return out
}

// imageTagIDs returns the tag ids linked to an image.
func imageTagIDs(imageID uint) []uint {
	var rows []struct {
		TagID uint
	}
	database.DB.Table("image_tags").Select("tag_id").
		Where("image_id = ?", imageID).Scan(&rows)
	out := make([]uint, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.TagID)
	}
	return out
}