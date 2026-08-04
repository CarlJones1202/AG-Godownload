package services

import (
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"math"
	"sort"
	"sync"
)

// ScoredImage is an image id with its normalized cosine similarity (0-1).
type ScoredImage struct {
	ImageID uint
	Score   float64
}

// VectorIndex is the query interface for image vectors. The default
// implementation is an in-memory brute-force index; a dedicated ANN index can
// be swapped in behind this interface without touching callers.
type VectorStore interface {
	Rebuild() error
	Upsert(imageID uint, vec []float32)
	Remove(imageID uint)
	Get(imageID uint) ([]float32, bool)
	SearchByVector(vec []float32, k int, exclude map[uint]bool) []ScoredImage
	Len() int
	Dim() int
}

// VectorIndex is the shared index instance.
var VectorIndex = &MemoryIndex{}

// MemoryIndex is a simple map-backed, RWMutex-protected index over normalized
// vectors. Vectors are normalized on insert, so cosine similarity is the dot
// product.
type MemoryIndex struct {
	mu       sync.RWMutex
	vecs     map[uint][]float32
	dim      int
	embedder string
}

func (m *MemoryIndex) ensure() {
	if m.vecs == nil {
		m.vecs = make(map[uint][]float32)
	}
}

func (m *MemoryIndex) Rebuild() error {
	m.mu.Lock()
	m.ensure()
	m.mu.Unlock()

	embedder := CurrentEmbedder().Name()

	type row struct {
		ImageID  uint
		Embedder string
		Vector   []byte
	}
	var rows []row
	if err := database.DB.Model(&models.ImageEmbedding{}).
		Select("image_embeddings.image_id, image_embeddings.embedder, image_embeddings.vector").
		Joins("JOIN images ON images.id = image_embeddings.image_id").
		Where("images.deleted_at IS NULL AND images.type = ? AND images.file_exists = ?", "image", true).
		Where("image_embeddings.embedder = ?", embedder).
		Find(&rows).Error; err != nil {
		return err
	}

	newMap := make(map[uint][]float32, len(rows))
	dim := 0
	for _, r := range rows {
		vec, err := vecFromBytes(r.Vector)
		if err != nil {
			logger.Warnf("Skipping malformed vector for image %d: %v", r.ImageID, err)
			continue
		}
		if dim == 0 {
			dim = len(vec)
		}
		newMap[r.ImageID] = vec
	}

	m.mu.Lock()
	m.vecs = newMap
	m.dim = dim
	m.embedder = embedder
	m.mu.Unlock()

	logger.Infof("Vector index rebuilt: %d vectors (%s, dim %d)", len(newMap), embedder, dim)
	return nil
}

func (m *MemoryIndex) Upsert(imageID uint, vec []float32) {
	m.mu.Lock()
	m.ensure()
	m.vecs[imageID] = vec
	if m.dim == 0 {
		m.dim = len(vec)
	}
	m.mu.Unlock()
}

func (m *MemoryIndex) Remove(imageID uint) {
	m.mu.Lock()
	delete(m.vecs, imageID)
	m.mu.Unlock()
}

func (m *MemoryIndex) Get(imageID uint) ([]float32, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.vecs[imageID]
	return v, ok
}

// SearchByVector returns the top-k vectors most similar to vec (normalized
// cosine). Candidate ids in exclude are skipped.
func (m *MemoryIndex) SearchByVector(vec []float32, k int, exclude map[uint]bool) []ScoredImage {
	query := normalizeVec(vec)
	if query == nil {
		return nil
	}

	m.mu.RLock()
	results := make([]ScoredImage, 0, len(m.vecs))
	for id, v := range m.vecs {
		if exclude != nil && exclude[id] {
			continue
		}
		if len(v) != len(query) {
			continue
		}
		sim := dotProduct(query, v)
		if sim < 0 {
			sim = 0
		}
		results = append(results, ScoredImage{ImageID: id, Score: sim})
	}
	m.mu.RUnlock()

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].ImageID < results[j].ImageID
		}
		return results[i].Score > results[j].Score
	})

	if k > 0 && len(results) > k {
		results = results[:k]
	}
	return results
}

func (m *MemoryIndex) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.vecs)
}

func (m *MemoryIndex) Dim() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.dim
}

func dotProduct(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

func cosineVec(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	sim := dotProduct(a, b)
	if sim < 0 {
		sim = 0
	}
	if sim > 1 {
		sim = 1
	}
	n := math.Sqrt(dotProduct(a, a) * dotProduct(b, b))
	if n < 1e-12 {
		return 0
	}
	return sim / n
}