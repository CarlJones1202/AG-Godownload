package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/models"
	"math"
	"sort"
)

// ErrSeedNotEmbedded is returned when the seed image has no vector yet.
var ErrSeedNotEmbedded = errors.New("image is not embedded yet")

// Recommendation is one ranked result with human-readable reasons.
type Recommendation struct {
	Image     models.Image `json:"image"`
	Score     float64      `json:"similarity"`
	Reasons   []string     `json:"reasons"`
	GalleryID uint         `json:"gallery_id,omitempty"`
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
	sources   []uint
	tagIDs    []uint
	tagNames  []string
	// seedMatches is how many seeds this candidate visibly resembles
	// (cosine >= seedMatchThreshold); used for multi-seed "reasons".
	seedMatches int
}

type seedVector struct {
	id  uint
	vec []float32
}

// seedMatchThreshold is the cosine similarity at which a candidate counts as
// resembling one of the consensus seeds.
const seedMatchThreshold = 0.55
const maxIndividualSeedSearches = 8

// Consensus describes the common ground derived from a set of seed images:
// which seeds had embeddings, their centrality weights, and the tags shared
// enough to count as "consensus".
type Consensus struct {
	Weights        map[uint]float64 // seed id -> centrality weight (not normalized)
	EmbeddedIDs    []uint           // seeds that had vectors (in input order)
	SkippedIDs     []uint           // seeds without vectors
	CommonTags     map[uint]float64 // tag id -> aggregate weight across seeds
	CommonTagNames []string
}

// RecommendSimilar returns ranked recommendations for a seed image.
func RecommendSimilar(seedID uint, limit int) ([]Recommendation, error) {
	recs, _, err := RecommendFromSeeds([]uint{seedID}, limit)
	return recs, err
}

// centralityWeights assigns each seed a weight based on its similarity to the
// mean of the other seeds, soft-floored so outliers keep a small voice instead
// of being dropped (or dominating). This keeps weighting linear in the number
// of seeds rather than comparing every pair. A single seed gets weight 1.
func centralityWeights(vecs [][]float32) []float64 {
	n := len(vecs)
	weights := make([]float64, n)
	if n == 0 {
		return weights
	}
	if n == 1 {
		weights[0] = 1
		return weights
	}
	dim := len(vecs[0])
	sum := make([]float64, dim)
	for _, v := range vecs {
		if len(v) != dim {
			continue
		}
		for i, x := range v {
			sum[i] += float64(x)
		}
	}
	const floor = 0.15
	for i, v := range vecs {
		if len(v) != dim {
			weights[i] = floor
			continue
		}
		others := make([]float32, dim)
		for j, x := range v {
			others[j] = float32((sum[j] - float64(x)) / float64(n-1))
		}
		c := cosineVec(v, others)
		if c < floor {
			c = floor
		}
		weights[i] = c
	}
	return weights
}

// RecommendFromSeeds returns ranked recommendations for a *set* of seeds,
// blended against the consensus of the set rather than each seed individually:
// a centrality-weighted centroid of the seed embeddings, tags shared across
// seeds, and a merged color palette. Seeds without embeddings are skipped and
// reported in the Consensus. With a single seed this degenerates to the
// original single-seed behavior.
func RecommendFromSeeds(seedIDs []uint, limit int) ([]Recommendation, *Consensus, error) {
	if VectorIndex.Len() == 0 {
		return nil, nil, errors.New("no embeddings indexed yet")
	}
	if limit <= 0 || limit > 200 {
		limit = 24
	}

	// Load + normalize seed vectors; track which seeds are usable.
	exclude := make(map[uint]bool, len(seedIDs))
	consensus := &Consensus{
		Weights:    map[uint]float64{},
		CommonTags: map[uint]float64{},
	}
	var embedded []seedVector
	for _, id := range seedIDs {
		exclude[id] = true
		raw, err := LoadVector(id)
		if err != nil || raw == nil {
			consensus.SkippedIDs = append(consensus.SkippedIDs, id)
			continue
		}
		nv := normalizeVec(raw)
		if nv == nil {
			consensus.SkippedIDs = append(consensus.SkippedIDs, id)
			continue
		}
		embedded = append(embedded, seedVector{id: id, vec: nv})
		consensus.EmbeddedIDs = append(consensus.EmbeddedIDs, id)
	}
	if len(embedded) == 0 {
		return nil, consensus, fmt.Errorf("%w: none of the %d seed images have vectors", ErrSeedNotEmbedded, len(seedIDs))
	}
	dislikedIDs, err := DefaultProfile.DislikedImageIDs()
	if err != nil {
		return nil, consensus, fmt.Errorf("load disliked images: %w", err)
	}
	for _, id := range dislikedIDs {
		exclude[id] = true
	}
	likedIDs, err := DefaultProfile.LikedImageIDs()
	if err != nil {
		return nil, nil, fmt.Errorf("load liked images: %w", err)
	}
	for _, id := range likedIDs {
		exclude[id] = true
	}

	// Centrality weighting: a seed that resembles the rest of the set gets a
	// big vote; an outlier favorite is soft-floored rather than dropped, so it
	// tugs the consensus without hijacking it.
	vecs := make([][]float32, 0, len(embedded))
	for _, s := range embedded {
		vecs = append(vecs, s.vec)
	}
	weights := centralityWeights(vecs)
	for i, s := range embedded {
		consensus.Weights[s.id] = weights[i]
	}
	wSum := 0.0
	for _, w := range consensus.Weights {
		wSum += w
	}

	// Consensus vector: the weighted centroid of normalized seed vectors.
	// SearchByVector renormalizes, so no need here.
	var centroid []float32
	for _, s := range embedded {
		w := float32(consensus.Weights[s.id] / wSum)
		if centroid == nil {
			centroid = make([]float32, len(s.vec))
		}
		for i, v := range s.vec {
			centroid[i] += w * v
		}
	}

	// Consensus tags: aggregate seed weights per tag; keep tags carried by a
	// meaningful share of the set (a lone seed's tags still pass at weight 1).
	tagMap := bulkImageTagData(consensus.EmbeddedIDs)
	for _, s := range embedded {
		w := consensus.Weights[s.id] / wSum
		for _, tid := range tagMap[s.id].ids {
			consensus.CommonTags[tid] += w
		}
	}
	const tagThreshold = 0.25
	nameByTagID := map[uint]string{}
	for _, s := range embedded {
		d := tagMap[s.id]
		for i, tid := range d.ids {
			nameByTagID[tid] = d.names[i]
		}
	}
	for tid, w := range consensus.CommonTags {
		if w < tagThreshold {
			delete(consensus.CommonTags, tid)
			continue
		}
		consensus.CommonTagNames = append(consensus.CommonTagNames, nameByTagID[tid])
	}

	// Merged color palette: union of every embedded seed palette, exact-hex deduped.
	seedColorMap := bulkImageColors(consensus.EmbeddedIDs)
	seenColors := map[string]bool{}
	var mergedPalette []string
	for _, id := range consensus.EmbeddedIDs {
		for _, hex := range seedColorMap[id] {
			if seenColors[hex] {
				continue
			}
			seenColors[hex] = true
			mergedPalette = append(mergedPalette, hex)
		}
	}

	profileReady := DefaultProfile.Ready()
	prefVec := DefaultProfile.Vector()
	affinities := DefaultProfile.TagAffinity()

	poolSize := limit*6 + 48
	candidateIDs := make(map[uint]struct{}, poolSize*(maxIndividualSeedSearches+2))
	addSearchResults := func(query []float32) {
		for _, result := range VectorIndex.SearchByVector(query, poolSize, exclude) {
			candidateIDs[result.ImageID] = struct{}{}
		}
	}
	addSearchResults(centroid)

	// Individual seed searches recover matches to a favorite that may be far
	// from the overall centroid. Sample evenly across centrality-ranked seeds
	// so both representative favorites and outliers can contribute candidates.
	for _, s := range representativeSeedVectors(embedded, weights, maxIndividualSeedSearches) {
		addSearchResults(s)
	}
	if profileReady && prefVec != nil {
		addSearchResults(prefVec)
	}

	commonTagIDs := make([]uint, 0, len(consensus.CommonTags))
	for id := range consensus.CommonTags {
		commonTagIDs = append(commonTagIDs, id)
	}
	sort.Slice(commonTagIDs, func(i, j int) bool { return commonTagIDs[i] < commonTagIDs[j] })
	tagCandidateIDs, err := findTaggedCandidates(commonTagIDs, exclude, poolSize)
	if err != nil {
		return nil, consensus, fmt.Errorf("find tag-based candidates: %w", err)
	}
	for _, id := range tagCandidateIDs {
		candidateIDs[id] = struct{}{}
	}
	if len(candidateIDs) == 0 {
		return nil, consensus, errors.New("no candidates found")
	}

	ids := make([]uint, 0, len(candidateIDs))
	for id := range candidateIDs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	galMap := bulkImageGalleryIDs(ids)
	sourceMap := bulkImageSourceIDs(ids)
	tagDataMap := bulkImageTagData(ids)
	colorMap := bulkImageColors(ids)
	vecMap := make(map[uint][]float32, len(ids))
	validIDs := ids[:0]
	for _, id := range ids {
		if v, ok := VectorIndex.Get(id); ok {
			vecMap[id] = v
			validIDs = append(validIDs, id)
		}
	}

	weight := config.Global.Embedding.RecWeights

	cands := make([]candidate, 0, len(validIDs))
	maxSeedWeight := 0.0
	for _, w := range weights {
		if w > maxSeedWeight {
			maxSeedWeight = w
		}
	}
	for _, id := range validIDs {
		vec := vecMap[id]
		vecSim := candidateSeedSimilarity(centroid, vec, embedded, weights, maxSeedWeight)
		c := candidate{
			imageID:   id,
			vec:       vec,
			vecSim:    vecSim,
			galleries: galMap[id],
			sources:   sourceMap[id],
			tagIDs:    tagDataMap[id].ids,
			tagNames:  tagDataMap[id].names,
		}
		c.tagSim = computeTagSimWeighted(consensus.CommonTags, c.tagIDs, affinities)
		c.colorSim = paletteSimilarity(mergedPalette, colorMap[id])
		if profileReady && prefVec != nil {
			sim := dotProduct(prefVec, c.vec)
			if sim < 0 {
				sim = 0
			}
			if sim > 1 {
				sim = 1
			}
			c.prefSim = sim
		}
		if len(embedded) > 1 && c.vec != nil {
			for _, s := range embedded {
				if cosineVec(s.vec, c.vec) >= seedMatchThreshold {
					c.seedMatches++
				}
			}
		}
		c.score = weight[0]*c.vecSim + weight[1]*c.tagSim + weight[2]*c.colorSim + weight[3]*c.prefSim
		cands = append(cands, c)
	}

	// Never recommend from any seed's own gallery (union across the set).
	var seedGalleries []uint
	seedGalRows := bulkImageGalleryIDs(consensus.EmbeddedIDs)
	galSeen := map[uint]bool{}
	for _, gs := range seedGalRows {
		for _, g := range gs {
			if !galSeen[g] {
				galSeen[g] = true
				seedGalleries = append(seedGalleries, g)
			}
		}
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
		return nil, consensus, err
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
			Image:     img,
			Score:     c.score,
			Reasons:   buildReasons(&c, consensus.CommonTagNames),
			GalleryID: firstGalleryID(c.galleries),
		})
	}
	return recs, consensus, nil
}

func candidateSeedSimilarity(centroid, vec []float32, seeds []seedVector, weights []float64, maxWeight float64) float64 {
	similarity := cosineVec(centroid, vec)
	if maxWeight <= 0 {
		return similarity
	}
	for i, seed := range seeds {
		seedSimilarity := cosineVec(seed.vec, vec) * weights[i] / maxWeight
		if seedSimilarity > similarity {
			similarity = seedSimilarity
		}
	}
	return similarity
}

func firstGalleryID(ids []uint) uint {
	if len(ids) == 0 {
		return 0
	}
	return ids[0]
}

func representativeSeedVectors(seeds []seedVector, weights []float64, limit int) [][]float32 {
	if limit <= 0 || len(seeds) == 0 {
		return nil
	}
	if len(seeds) <= limit {
		out := make([][]float32, len(seeds))
		for i, seed := range seeds {
			out[i] = seed.vec
		}
		return out
	}

	indices := make([]int, len(seeds))
	for i := range indices {
		indices[i] = i
	}
	sort.Slice(indices, func(i, j int) bool {
		left, right := indices[i], indices[j]
		if weights[left] == weights[right] {
			return left < right
		}
		return weights[left] < weights[right]
	})

	out := make([][]float32, 0, limit)
	if limit == 1 {
		return append(out, seeds[indices[len(indices)-1]].vec)
	}
	for i := 0; i < limit; i++ {
		index := i * (len(indices) - 1) / (limit - 1)
		out = append(out, seeds[indices[index]].vec)
	}
	return out
}

func findTaggedCandidates(tagIDs []uint, exclude map[uint]bool, limit int) ([]uint, error) {
	if len(tagIDs) == 0 || limit <= 0 {
		return nil, nil
	}
	var rows []struct {
		ImageID uint
	}
	err := database.DB.Table("image_tags").
		Select("image_tags.image_id").
		Joins("JOIN images ON images.id = image_tags.image_id").
		Where("image_tags.tag_id IN ? AND images.type = ? AND images.file_exists = ?", tagIDs, "image", true).
		Where("images.deleted_at IS NULL").
		Group("image_tags.image_id").
		Order("COUNT(DISTINCT image_tags.tag_id) DESC, image_tags.image_id ASC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		if !exclude[row.ImageID] {
			ids = append(ids, row.ImageID)
		}
	}
	return ids, nil
}

// selectMMR greedily picks the most relevant candidates while penalizing
// similarity to already-chosen ones and clustering from the same gallery.
func selectMMR(cands []candidate, limit int, seedGalleries []uint) []candidate {
	lambda := config.Global.Embedding.RecDiversityLambda
	const (
		maxPerGallery = 2
		maxPerSource  = 3
	)

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
	sourceCounts := make(map[uint]int)
	var chosenVecs [][]float32
	var chosenTags [][]uint

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
			for _, tags := range chosenTags {
				if overlap := tagJaccard(c.tagIDs, tags); overlap > div {
					div = overlap
				}
			}
			galleryPenalty := 0.0
			for _, g := range c.galleries {
				if n := galleryCounts[g]; n >= maxPerGallery {
					galleryPenalty += 0.25 * float64(n-maxPerGallery+1)
				}
			}
			sourcePenalty := 0.0
			for _, source := range c.sources {
				if n := sourceCounts[source]; n >= maxPerSource {
					sourcePenalty += 0.12 * float64(n-maxPerSource+1)
				}
			}
			val := (1-lambda)*c.score - lambda*div - galleryPenalty - sourcePenalty
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
		chosenTags = append(chosenTags, best.tagIDs)
		for _, g := range best.galleries {
			galleryCounts[g]++
		}
		for _, source := range best.sources {
			sourceCounts[source]++
		}
	}
	out := make([]candidate, 0, len(chosen))
	for _, c := range chosen {
		out = append(out, *c)
	}
	return out
}

func tagJaccard(a, b []uint) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	aSet := make(map[uint]struct{}, len(a))
	for _, id := range a {
		aSet[id] = struct{}{}
	}
	intersection := 0
	union := len(aSet)
	bSeen := make(map[uint]struct{}, len(b))
	for _, id := range b {
		if _, ok := bSeen[id]; ok {
			continue
		}
		bSeen[id] = struct{}{}
		if _, ok := aSet[id]; ok {
			intersection++
		} else {
			union++
		}
	}
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// computeTagSimWeighted scores tag overlap between the consensus tag set and a
// candidate, weighted by the user's per-tag affinity (default +0.5 when the
// profile has no opinion). Consensus weights are aggregate seed shares, so a
// tag carried by most favorites counts more than one carried by an outlier.
func computeTagSimWeighted(consensusTags map[uint]float64, candIDs []uint, affinities map[uint]float64) float64 {
	if len(consensusTags) == 0 || len(candIDs) == 0 {
		return 0
	}
	total := 0.0
	for _, id := range candIDs {
		if w, ok := consensusTags[id]; ok {
			a := affinities[id]
			if a == 0 {
				a = 0.5
			}
			if a > 1 {
				a = 1
			}
			total += a * w
		}
	}
	if total < 0 {
		total = 0
	}
	norm := math.Sqrt(float64(len(consensusTags)))
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
	candidateColors := make([][3]int, 0, len(cand))
	for _, hex := range cand {
		r, g, b, err := HexToRGB(hex)
		if err == nil {
			candidateColors = append(candidateColors, [3]int{r, g, b})
		}
	}
	sum := 0.0
	count := 0
	for _, sc := range seed {
		r, g, b, err := HexToRGB(sc)
		if err != nil || len(candidateColors) == 0 {
			continue
		}
		distance := math.MaxFloat64
		for _, color := range candidateColors {
			d := ColorDistance(r, g, b, color[0], color[1], color[2])
			if d < distance {
				distance = d
			}
		}
		sum += CalculateColorSimilarity(distance) / 100.0
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func buildReasons(c *candidate, consensusTags []string) []string {
	var reasons []string
	if c.seedMatches > 1 {
		reasons = append(reasons, fmt.Sprintf("matches %d favorites", c.seedMatches))
	}
	if c.vecSim >= seedMatchThreshold {
		reasons = append(reasons, "similar look")
	}
	if c.colorSim >= 0.6 {
		reasons = append(reasons, "similar colors")
	}
	for _, t := range c.tagNames {
		if len(reasons) >= 4 {
			break
		}
		if containsString(consensusTags, t) {
			reasons = append(reasons, "tag: "+t)
		}
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "content match")
	}
	return reasons
}

// --- bulk data loaders ---

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

type imageSourceRow struct {
	ImageID  uint
	SourceID uint
}

func bulkImageSourceIDs(ids []uint) map[uint][]uint {
	out := make(map[uint][]uint)
	if len(ids) == 0 {
		return out
	}
	var rows []imageSourceRow
	database.DB.Table("image_galleries").
		Select("image_galleries.image_id, galleries.source_id").
		Joins("JOIN galleries ON galleries.id = image_galleries.gallery_id").
		Where("image_galleries.image_id IN ? AND galleries.source_id IS NOT NULL", ids).
		Scan(&rows)
	seen := make(map[uint]map[uint]bool)
	for _, r := range rows {
		if seen[r.ImageID] == nil {
			seen[r.ImageID] = make(map[uint]bool)
		}
		if seen[r.ImageID][r.SourceID] {
			continue
		}
		seen[r.ImageID][r.SourceID] = true
		out[r.ImageID] = append(out[r.ImageID], r.SourceID)
	}
	return out
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

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
