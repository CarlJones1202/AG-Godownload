package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/models"
	"math"
)

// ErrSeedNotEmbedded is returned when the seed image has no vector yet.
var ErrSeedNotEmbedded = errors.New("image is not embedded yet")

// Recommendation is one ranked result with human-readable reasons.
type Recommendation struct {
	Image   models.Image `json:"image"`
	Score   float64      `json:"similarity"`
	Reasons []string     `json:"reasons"`
}

type candidate struct {
	imageID   uint
	vec       []float32
	vecSim    float64
	tagSim    float64
	colorSim  float64
	prefSim   float64
	score     float64
	galleries []uint
	tagIDs    []uint
	tagNames  []string
}

// RecommendSimilar returns ranked recommendations for a seed image, blending
// embedding similarity, tag affinity, color similarity and (once the profile
// has enough signal) the personalized pull, then applying MMR-style diversity.
func RecommendSimilar(seedID uint, limit int) ([]Recommendation, error) {
	if VectorIndex.Len() == 0 {
		return nil, errors.New("no embeddings indexed yet")
	}
	seedVec, err := LoadVector(seedID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSeedNotEmbedded, err)
	}
	if limit <= 0 || limit > 200 {
		limit = 24
	}

	exclude := map[uint]bool{seedID: true}

	seedGalleries := imageGalleryIDs(seedID)
	seedTagIDs, seedTagNames := imageTagData(seedID)
	seedColors := parseColorPalette(seedColorJSON(seedID))

	profileReady := DefaultProfile.Ready()
	prefVec := DefaultProfile.Vector()
	affinities := DefaultProfile.TagAffinity()

	poolSize := limit*6 + 48
	pool := VectorIndex.SearchByVector(seedVec, poolSize, exclude)
	if len(pool) == 0 {
		return nil, errors.New("no candidates found")
	}

	ids := make([]uint, 0, len(pool))
	for _, sc := range pool {
		ids = append(ids, sc.ImageID)
	}
	galMap := bulkImageGalleryIDs(ids)
	tagMap := bulkImageTagData(ids)
	colorMap := bulkImageColors(ids)
	vecMap := make(map[uint][]float32, len(ids))
	for _, sc := range pool {
		if v, ok := VectorIndex.Get(sc.ImageID); ok {
			vecMap[sc.ImageID] = v
		}
	}

	weight := config.Global.Embedding.RecWeights

	cands := make([]candidate, 0, len(pool))
	for _, sc := range pool {
		c := candidate{
			imageID:   sc.ImageID,
			vec:       vecMap[sc.ImageID],
			vecSim:    sc.Score,
			galleries: galMap[sc.ImageID],
			tagIDs:    tagMap[sc.ImageID].ids,
			tagNames:  tagMap[sc.ImageID].names,
		}
		c.tagSim = computeTagSim(seedTagIDs, c.tagIDs, affinities)
		c.colorSim = paletteSimilarity(seedColors, colorMap[sc.ImageID])
		if profileReady && prefVec != nil && c.vec != nil {
			sim := dotProduct(prefVec, c.vec)
			if sim < 0 {
				sim = 0
			}
			c.prefSim = sim
		}
		c.score = weight[0]*c.vecSim + weight[1]*c.tagSim + weight[2]*c.colorSim + weight[3]*c.prefSim
		cands = append(cands, c)
	}

	chosen := selectMMR(cands, limit, seedGalleries)

	recs := make([]Recommendation, 0, len(chosen))
	chosenIDs := make([]uint, 0, len(chosen))
	for _, c := range chosen {
		chosenIDs = append(chosenIDs, c.imageID)
	}
	var images []models.Image
	if err := database.DB.Select("id, filename, dominant_colors, type, file_exists, created_at").
		Where("id IN ?", chosenIDs).Find(&images).Error; err != nil {
		return nil, err
	}
	imgMap := make(map[uint]models.Image, len(images))
	for _, im := range images {
		imgMap[im.ID] = im
	}
	for _, c := range chosen {
		img, ok := imgMap[c.imageID]
		if !ok {
			continue
		}
		recs = append(recs, Recommendation{
			Image:   img,
			Score:   c.score,
			Reasons: buildReasons(&c, seedTagNames),
		})
	}
	return recs, nil
}

// selectMMR greedily picks the most relevant candidates while penalizing
// similarity to already-chosen ones and clustering from the same gallery.
func selectMMR(cands []candidate, limit int, seedGalleries []uint) []candidate {
	lambda := config.Global.Embedding.RecDiversityLambda
	const maxPerGallery = 2

	seedGalSet := make(map[uint]bool, len(seedGalleries))
	for _, g := range seedGalleries {
		seedGalSet[g] = true
	}

	remaining := make([]*candidate, 0, len(cands))
	for i := range cands {
		c := &cands[i]
		// Never recommend images from the seed's own gallery.
		sameGallery := false
		for _, g := range c.galleries {
			if seedGalSet[g] {
				sameGallery = true
				break
			}
		}
		if !sameGallery {
			remaining = append(remaining, c)
		}
	}

	var chosen []*candidate
	galleryCounts := make(map[uint]int)
	var chosenVecs [][]float32

	for len(chosen) < limit && len(remaining) > 0 {
		bestIdx := -1
		bestVal := math.Inf(-1)
		for i, c := range remaining {
			div := 0.0
			if len(chosenVecs) > 0 && c.vec != nil {
				for _, v := range chosenVecs {
					s := cosineVec(v, c.vec)
					if s > div {
						div = s
					}
				}
			}
			galleryPenalty := 0.0
			for _, g := range c.galleries {
				if n := galleryCounts[g]; n >= maxPerGallery {
					galleryPenalty += 0.25 * float64(n-maxPerGallery+1)
				}
			}
			val := (1-lambda)*c.score - lambda*div - galleryPenalty
			if val > bestVal {
				bestVal = val
				bestIdx = i
			}
		}
		if bestIdx < 0 {
			break
		}
		best := remaining[bestIdx]
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
		chosen = append(chosen, best)
		if best.vec != nil {
			chosenVecs = append(chosenVecs, best.vec)
		}
		for _, g := range best.galleries {
			galleryCounts[g]++
		}
	}
	out := make([]candidate, 0, len(chosen))
	for _, c := range chosen {
		out = append(out, *c)
	}
	return out
}

// computeTagSim scores tag overlap between seed and candidate, weighted by the
// user's per-tag affinity (default +0.5 when the profile has no opinion).
func computeTagSim(seedIDs []uint, candIDs []uint, affinities map[uint]float64) float64 {
	if len(seedIDs) == 0 || len(candIDs) == 0 {
		return 0
	}
	total := 0.0
	for _, id := range candIDs {
		if containsUint(seedIDs, id) {
			a := affinities[id]
			if a == 0 {
				a = 0.5
			}
			if a > 1 {
				a = 1
			}
			total += a
		}
	}
	if total < 0 {
		total = 0
	}
	norm := math.Sqrt(float64(len(seedIDs)))
	if norm == 0 {
		return 0
	}
	return total / norm
}

// paletteSimilarity averages the per-color similarity between two dominant
// color palettes (0-1, 1 = identical palette).
func paletteSimilarity(seed, cand []string) float64 {
	if len(seed) == 0 || len(cand) == 0 {
		return 0
	}
	paletteJSON, err := json.Marshal(cand)
	if err != nil {
		return 0
	}
	sum := 0.0
	count := 0
	for _, sc := range seed {
		distance, _, err := FindSimilarColorInPalette(sc, string(paletteJSON))
		if err != nil {
			continue
		}
		sum += CalculateColorSimilarity(distance) / 100.0
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func buildReasons(c *candidate, seedTags []string) []string {
	var reasons []string
	if c.vecSim >= 0.55 {
		reasons = append(reasons, "similar look")
	}
	if c.colorSim >= 0.6 {
		reasons = append(reasons, "similar colors")
	}
	for _, t := range c.tagNames {
		if len(reasons) >= 4 {
			break
		}
		if containsString(seedTags, t) {
			reasons = append(reasons, "tag: "+t)
		}
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "content match")
	}
	return reasons
}

// --- bulk data loaders ---

func imageGalleryIDs(imageID uint) []uint {
	rows := bulkImageGalleryIDs([]uint{imageID})
	return rows[imageID]
}

func imageTagData(imageID uint) ([]uint, []string) {
	m := bulkImageTagData([]uint{imageID})
	d := m[imageID]
	return d.ids, d.names
}

func seedColorJSON(imageID uint) string {
	var im models.Image
	if err := database.DB.Select("dominant_colors").First(&im, imageID).Error; err != nil {
		return ""
	}
	return im.DominantColors
}

func parseColorPalette(jsonStr string) []string {
	if jsonStr == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(jsonStr), &out); err != nil {
		return nil
	}
	return out
}

type imageTagDataRow struct {
	ImageID uint
	TagID   uint
	Name    string
}

func bulkImageTagData(ids []uint) map[uint]struct {
	ids   []uint
	names []string
} {
	out := make(map[uint]struct {
		ids   []uint
		names []string
	})
	if len(ids) == 0 {
		return out
	}
	var rows []imageTagDataRow
	database.DB.Table("image_tags").
		Select("image_tags.image_id, tags.id AS tag_id, tags.name").
		Joins("JOIN tags ON tags.id = image_tags.tag_id").
		Where("image_tags.image_id IN ?", ids).
		Scan(&rows)
	for _, r := range rows {
		d := out[r.ImageID]
		d.ids = append(d.ids, r.TagID)
		d.names = append(d.names, r.Name)
		out[r.ImageID] = d
	}
	return out
}

type imageGalleryRow struct {
	ImageID   uint
	GalleryID uint
}

func bulkImageGalleryIDs(ids []uint) map[uint][]uint {
	out := make(map[uint][]uint)
	if len(ids) == 0 {
		return out
	}
	var rows []imageGalleryRow
	database.DB.Table("image_galleries").
		Select("image_id, gallery_id").
		Where("image_id IN ?", ids).
		Scan(&rows)
	for _, r := range rows {
		out[r.ImageID] = append(out[r.ImageID], r.GalleryID)
	}
	return out
}

func bulkImageColors(ids []uint) map[uint][]string {
	out := make(map[uint][]string)
	if len(ids) == 0 {
		return out
	}
	var rows []models.Image
	database.DB.Select("id, dominant_colors").
		Where("id IN ?", ids).Find(&rows)
	for _, r := range rows {
		out[r.ID] = parseColorPalette(r.DominantColors)
	}
	return out
}

func containsUint(list []uint, v uint) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}