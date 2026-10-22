package services

import (
	"math"
	"testing"
)

func unitVec(vals ...float64) []float32 {
	out := make([]float32, len(vals))
	var sum float64
	for _, v := range vals {
		sum += v * v
	}
	n := math.Sqrt(sum)
	for i, v := range vals {
		out[i] = float32(v / n)
	}
	return out
}

func TestCentralityWeightsSingleSeed(t *testing.T) {
	w := centralityWeights([][]float32{unitVec(1, 0)})
	if len(w) != 1 || w[0] != 1 {
		t.Fatalf("single seed should get weight 1, got %v", w)
	}
}

func TestCentralityWeightsOutlierDownWeighted(t *testing.T) {
	// Three near-identical seeds and one orthogonal outlier.
	a := unitVec(1, 0, 0)
	b := unitVec(1, 0.1, 0)
	c := unitVec(1, -0.1, 0)
	outlier := unitVec(0, 0, 1)

	w := centralityWeights([][]float32{a, b, c, outlier})
	for i := 0; i < 3; i++ {
		if w[i] <= w[3] {
			t.Fatalf("cluster seeds should outrank the outlier: %v", w)
		}
	}
	// Outlier is soft-floored, not zeroed — it keeps a small voice.
	if w[3] <= 0 {
		t.Fatalf("outlier should keep a positive floor weight, got %v", w[3])
	}
}

func TestCentralityWeightsCoherentSetEven(t *testing.T) {
	w := centralityWeights([][]float32{
		unitVec(1, 0, 0),
		unitVec(1, 0.05, 0),
		unitVec(1, -0.05, 0),
	})
	for i, v := range w {
		if math.Abs(v-w[0]) > 1e-9 {
			t.Fatalf("nearly-identical seeds should weight evenly, index %d: %v", i, w)
		}
	}
}

func TestComputeTagSimWeightedSharedTagCounts(t *testing.T) {
	affinities := map[uint]float64{}
	// Tag 1 carried by most of the set (weight 0.8), tag 2 by an outlier (0.2).
	consensus := map[uint]float64{1: 0.8, 2: 0.2}

	sharedOnly := computeTagSimWeighted(consensus, []uint{1}, affinities)
	outlierOnly := computeTagSimWeighted(consensus, []uint{2}, affinities)
	if sharedOnly <= outlierOnly {
		t.Fatalf("candidate with the widely-shared tag should score higher: %f vs %f", sharedOnly, outlierOnly)
	}
	none := computeTagSimWeighted(consensus, []uint{99}, affinities)
	if none != 0 {
		t.Fatalf("no overlap should score 0, got %f", none)
	}
}

func TestComputeTagSimWeightedAffinityBoost(t *testing.T) {
	consensus := map[uint]float64{1: 1}
	base := computeTagSimWeighted(consensus, []uint{1}, map[uint]float64{})
	boosted := computeTagSimWeighted(consensus, []uint{1}, map[uint]float64{1: 1})
	if boosted <= base {
		t.Fatalf("positive affinity should boost the tag score: %f vs %f", boosted, base)
	}
}

func TestComputeTagSimWeightedEmpty(t *testing.T) {
	if v := computeTagSimWeighted(nil, []uint{1}, nil); v != 0 {
		t.Fatalf("empty consensus should score 0, got %f", v)
	}
	if v := computeTagSimWeighted(map[uint]float64{1: 1}, nil, nil); v != 0 {
		t.Fatalf("empty candidate tags should score 0, got %f", v)
	}
}
