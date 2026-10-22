package services

import (
	"gallery_api/config"
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
		if math.Abs(v-w[0]) > 0.01 {
			t.Fatalf("nearly-identical seeds should receive similar weights, index %d: %v", i, w)
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

func TestPaletteSimilarityUsesEverySeedColor(t *testing.T) {
	sim := paletteSimilarity([]string{"#FF0000", "#00FF00"}, []string{"#FF0000"})
	if sim >= 1 || sim <= 0 {
		t.Fatalf("expected both seed colors to contribute, got %f", sim)
	}
}

func TestTagJaccard(t *testing.T) {
	if got := tagJaccard([]uint{1, 2, 2}, []uint{2, 3}); math.Abs(got-1.0/3.0) > 1e-9 {
		t.Fatalf("expected unique-tag Jaccard overlap 1/3, got %f", got)
	}
	if got := tagJaccard(nil, []uint{1}); got != 0 {
		t.Fatalf("empty tags should have no overlap, got %f", got)
	}
}

func TestRepresentativeSeedVectorsCoverCentralityRange(t *testing.T) {
	seeds := make([]seedVector, 12)
	weights := make([]float64, len(seeds))
	for i := range seeds {
		seeds[i] = seedVector{id: uint(i + 1), vec: unitVec(float64(i+1), 1)}
		weights[i] = float64(i + 1)
	}

	selected := representativeSeedVectors(seeds, weights, 4)
	if len(selected) != 4 || cosineVec(selected[0], seeds[0].vec) < 0.999 || cosineVec(selected[len(selected)-1], seeds[len(seeds)-1].vec) < 0.999 {
		t.Fatalf("expected representative searches to span least-to-most central seeds")
	}
}

func TestCandidateSeedSimilarityIncludesIndividualFavorites(t *testing.T) {
	centroid := unitVec(1, 1, 0)
	candidateVec := unitVec(0, 1, 0)
	seeds := []seedVector{
		{id: 1, vec: unitVec(1, 0, 0)},
		{id: 2, vec: unitVec(0, 1, 0)},
	}
	got := candidateSeedSimilarity(centroid, candidateVec, seeds, []float64{1, 1}, 1)
	if got < 0.99 {
		t.Fatalf("candidate matching an individual seed should retain that relevance, got %f", got)
	}
}

func TestSelectMMRPenalizesRepeatedSources(t *testing.T) {
	previous := config.Global.Embedding.RecDiversityLambda
	config.Global.Embedding.RecDiversityLambda = 0.6
	defer func() { config.Global.Embedding.RecDiversityLambda = previous }()

	selected := selectMMR([]candidate{
		{imageID: 1, score: 1, sources: []uint{10}},
		{imageID: 2, score: 0.9, sources: []uint{10}},
		{imageID: 3, score: 0.8, sources: []uint{10}},
		{imageID: 4, score: 0.2, sources: []uint{10}},
		{imageID: 5, score: 0.3, sources: []uint{20}},
	}, 4, nil)
	if len(selected) != 4 {
		t.Fatalf("expected all requested results despite source repetition, got %d", len(selected))
	}
	if selected[3].imageID != 5 {
		t.Fatalf("expected a new source to outrank a fourth result from the same source, got ids %d last", selected[3].imageID)
	}
}
