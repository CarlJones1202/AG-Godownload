package services

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"math"

	"gorm.io/gorm"
)

// EmbeddingResult bundles the vector and the analytic features produced for an
// image (features are always extracted, even by a semantic embedder).
type EmbeddingResult struct {
	Vector   []float32
	Features *LowLevelFeatures
}

// Embedder produces a normalized content vector for an image file.
type Embedder interface {
	Name() string // version tag stored in image_embeddings
	Dim() int
	Embed(path string) (*EmbeddingResult, error)
}

// activeEmbedder is selected once at startup by InitEmbedding().
var activeEmbedder Embedder

// LowLevelEmbedder is the built-in provider. It uses only analytic descriptors
// (color layout, vibrancy, contrast, edge density, ...) — no external model.
type LowLevelEmbedder struct {
	name string
	dim  int
}

func (e *LowLevelEmbedder) Name() string { return e.name }
func (e *LowLevelEmbedder) Dim() int      { return e.dim }

func (e *LowLevelEmbedder) Embed(path string) (*EmbeddingResult, error) {
	feats, err := ExtractLowLevelFeatures(path)
	if err != nil {
		return nil, fmt.Errorf("extract low-level features: %w", err)
	}
	vec := normalizeVec(feats.ToVector())
	if vec == nil {
		return nil, fmt.Errorf("zero-magnitude low-level vector for %s", path)
	}
	return &EmbeddingResult{Vector: vec, Features: feats}, nil
}

// OnnxEmbedder is the semantic embedding path. It is intentionally a stub
// until the Phase 0 spike validates a CLIP-style ONNX export on this machine.
type OnnxEmbedder struct {
	name      string
	dim       int
	modelPath string
}

func (e *OnnxEmbedder) Name() string { return e.name }
func (e *OnnxEmbedder) Dim() int      { return e.dim }

func (e *OnnxEmbedder) Embed(path string) (*EmbeddingResult, error) {
	return nil, fmt.Errorf("onnx embedder %q not implemented yet (Phase 0 spike required)", e.modelPath)
}

// InitEmbedding selects the active embedder based on config, then rebuilds the
// vector index. Should be called once at startup before starting the worker.
func InitEmbedding() error {
	cfg := config.Global.Embedding
	low := &LowLevelEmbedder{name: cfg.ModelName, dim: lowLevelVectorDim}
	if cfg.ModelPath != "" && cfg.Dim > 0 {
		logger.Warnf("EMBED_MODEL_PATH set (%s) but the ONNX embedder is not implemented; "+
			"falling back to low-level provider until Phase 0", cfg.ModelPath)
		activeEmbedder = low
	} else {
		activeEmbedder = low
	}
	logger.Infof("Embedding provider active: %s (dim %d)", activeEmbedder.Name(), activeEmbedder.Dim())

	if err := VectorIndex.Rebuild(); err != nil {
		return fmt.Errorf("failed to build vector index: %w", err)
	}
	return nil
}

func CurrentEmbedder() Embedder {
	if activeEmbedder == nil {
		activeEmbedder = &LowLevelEmbedder{name: config.Global.Embedding.ModelName, dim: lowLevelVectorDim}
	}
	return activeEmbedder
}

// StoreImageEmbedding writes (or overwrites) the embedding row for an image,
// updates its low-level feature JSON, and upserts it into the index.
// Returns the stored normalized vector.
func StoreImageEmbedding(imageID uint, res *EmbeddingResult) ([]float32, error) {
	vec := normalizeVec(res.Vector)
	if vec == nil {
		return nil, fmt.Errorf("refusing to store zero vector for image %d", imageID)
	}

	featJSON := ""
	if res.Features != nil {
		if b, err := json.Marshal(res.Features); err == nil {
			featJSON = string(b)
		}
	}

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("image_id = ?", imageID).Delete(&models.ImageEmbedding{}).Error; err != nil {
			return err
		}
		row := models.ImageEmbedding{
			ImageID:   imageID,
			Embedder:  CurrentEmbedder().Name(),
			Dimension: len(vec),
			Vector:    vecToBytes(vec),
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if featJSON != "" {
			if err := tx.Model(&models.Image{}).Where("id = ?", imageID).
				Update("image_content_features", featJSON).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	VectorIndex.Upsert(imageID, vec)
	return vec, nil
}

func vecToBytes(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:i*4+4], math.Float32bits(f))
	}
	return b
}

func vecFromBytes(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("vector blob length %d not divisible by 4", len(b))
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4 : i*4+4]))
	}
	return v, nil
}

// normalizeVec returns a unit vector, or nil if the input has ~zero magnitude.
func normalizeVec(v []float32) []float32 {
	var sum float64
	for _, f := range v {
		sum += float64(f) * float64(f)
	}
	norm := math.Sqrt(sum)
	if norm < 1e-9 {
		return nil
	}
	out := make([]float32, len(v))
	for i, f := range v {
		out[i] = float32(float64(f) / norm)
	}
	return out
}

// LoadVector returns the vector for an image, preferring the in-memory index.
func LoadVector(imageID uint) ([]float32, error) {
	if v, ok := VectorIndex.Get(imageID); ok {
		return v, nil
	}
	var row models.ImageEmbedding
	if err := database.DB.Where("image_id = ?", imageID).Order("id DESC").First(&row).Error; err != nil {
		return nil, err
	}
	return vecFromBytes(row.Vector)
}