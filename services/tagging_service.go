package services

import (
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
)

// DerivedTag is a (name, category) pair suggested for an image. Category maps
// into the existing tags.Category values ("mood", "color", "composition", ...).
type DerivedTag struct {
	Name     string
	Category string
}

// ImageTagNames returns the names of all tags linked to an image (any category).
func ImageTagNames(imageID uint) []string {
	var rows []struct{ Name string }
	if err := database.DB.Table("image_tags").
		Select("tags.name").
		Joins("JOIN tags ON tags.id = image_tags.tag_id").
		Where("image_tags.image_id = ?", imageID).
		Scan(&rows).Error; err != nil {
		logger.Warnf("ImageTagNames(%d) failed: %v", imageID, err)
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Name)
	}
	return out
}

// DeriveLowLevelTags turns analytic descriptors into human-meaningful tags.
// This is the Tier-B derived layer; a model-backed tagger can be added later
// to supply objects/people/pose tags without changing this pipeline.
func DeriveLowLevelTags(f *LowLevelFeatures) []DerivedTag {
	if f == nil {
		return nil
	}
	var tags []DerivedTag

	switch {
	case f.Vibrancy >= 0.55:
		tags = append(tags, DerivedTag{Name: "vibrant", Category: "mood"})
	case f.Vibrancy <= 0.25:
		tags = append(tags, DerivedTag{Name: "muted", Category: "mood"})
	}
	switch {
	case f.Brightness >= 0.60:
		tags = append(tags, DerivedTag{Name: "bright", Category: "mood"})
	case f.Brightness <= 0.35:
		tags = append(tags, DerivedTag{Name: "dark", Category: "mood"})
	}
	switch {
	case f.Contrast >= 0.22:
		tags = append(tags, DerivedTag{Name: "high-contrast", Category: "mood"})
	case f.Contrast <= 0.10:
		tags = append(tags, DerivedTag{Name: "soft-contrast", Category: "mood"})
	}
	switch {
	case f.EdgeDensity >= 0.30:
		tags = append(tags, DerivedTag{Name: "busy", Category: "composition"})
	case f.EdgeDensity <= 0.10:
		tags = append(tags, DerivedTag{Name: "minimal", Category: "composition"})
	}
	switch {
	case f.Aspect <= 0.80:
		tags = append(tags, DerivedTag{Name: "portrait-orientation", Category: "composition"})
	case f.Aspect >= 1.25:
		tags = append(tags, DerivedTag{Name: "landscape-orientation", Category: "composition"})
	}
	if f.Colorfulness >= 45 {
		tags = append(tags, DerivedTag{Name: "colorful", Category: "color"})
	}
	if f.Vibrancy <= 0.06 {
		tags = append(tags, DerivedTag{Name: "black-and-white", Category: "color"})
	}

	// Warm/cool judgment uses reddish vs blue-green hue bins.
	warm := f.HueHist[0] + f.HueHist[1] + f.HueHist[7] // 0-90 & 315-360
	cool := f.HueHist[4] + f.HueHist[5] + f.HueHist[6] // 180-315
	if warm > 0.45 && warm > cool*1.5 {
		tags = append(tags, DerivedTag{Name: "warm", Category: "color"})
	} else if cool > 0.45 && cool > warm*1.5 {
		tags = append(tags, DerivedTag{Name: "cool", Category: "color"})
	}

	return tags
}

// UpsertDerivedTags creates any new tags and links them to the image
// (idempotent — safe to call on re-embed).
func UpsertDerivedTags(imageID uint, derived []DerivedTag) error {
	if len(derived) == 0 {
		return nil
	}
	for _, d := range derived {
		var t models.Tag
		err := database.DB.Where("name = ?", d.Name).First(&t).Error
		if err != nil {
			t = models.Tag{Name: d.Name, Category: d.Category}
			if err := database.DB.Create(&t).Error; err != nil {
				// Likely a concurrent-created duplicate; re-fetch.
				if err2 := database.DB.Where("name = ?", d.Name).First(&t).Error; err2 != nil {
					logger.Debugf("Could not create tag %q: %v", d.Name, err)
					continue
				}
			}
		}
		if t.Category == "" {
			database.DB.Model(&t).Update("category", d.Category)
		}
		// Idempotent two-step insert (no GORM association overhead).
		var exists int64
		database.DB.Table("image_tags").
			Where("image_id = ? AND tag_id = ?", imageID, t.ID).
			Count(&exists)
		if exists == 0 {
			if err := database.DB.Exec(
				"INSERT INTO image_tags (image_id, tag_id) VALUES (?, ?)", imageID, t.ID).Error; err != nil {
				logger.Debugf("Failed to link tag %q to image %d: %v", d.Name, imageID, err)
			}
		}
	}
	return nil
}