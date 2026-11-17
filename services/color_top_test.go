package services

import (
	"encoding/json"
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"testing"
)

// mustPalette marshals colors into the JSON-array string stored in
// images.dominant_colors.
func mustPalette(t testing.TB, colors ...string) string {
	t.Helper()
	raw, err := json.Marshal(colors)
	if err != nil {
		t.Fatalf("failed to marshal palette: %v", err)
	}
	return string(raw)
}

// TestTopDominantColorsRanksDedupesAndAverages covers the core aggregation:
// near-identical hexes merge into one 4-bit bucket, an image counts at most
// once per bucket, malformed palettes/hexes and NULL/empty/[] rows are
// skipped, output is sorted by image count (descending), and the swatch color
// is the average of the member colors actually stored.
func TestTopDominantColorsRanksDedupesAndAverages(t *testing.T) {
	_, teardown := setupRecoveryTestDB(t, "top-colors-rank", false)
	defer teardown()

	seedImages(t, []models.Image{
		{Filename: "src/a.jpg", Type: "image", DominantColors: mustPalette(t, "#FF0000", "#FE0101", "#00FF00")},
		{Filename: "src/b.jpg", Type: "image", DominantColors: mustPalette(t, "#FF0000")},
		{Filename: "src/c.jpg", Type: "image", DominantColors: "not-json"},          // malformed palette: skipped
		{Filename: "src/d.jpg", Type: "image", DominantColors: mustPalette(t, "#ZZZZZZ")}, // invalid hex: skipped
		{Filename: "src/e.jpg", Type: "image", DominantColors: ""},                  // empty: excluded by query
		{Filename: "src/f.jpg", Type: "image", DominantColors: "[]"},                // empty array: excluded by query
		{Filename: "src/g.jpg", Type: "image"},                                      // NULL: excluded by query
	})
	if err := database.DB.Exec("UPDATE images SET dominant_colors = NULL WHERE filename = ?", "src/g.jpg").Error; err != nil {
		t.Fatalf("failed to null out dominant_colors: %v", err)
	}

	got, err := TopDominantColors(16)
	if err != nil {
		t.Fatalf("TopDominantColors failed: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 bucket entries, got %d: %+v", len(got), got)
	}

	// Red bucket: images a (deduped: #FF0000 + #FE0101) and b → 2 images.
	// Average of members (255+254+255)/3 = 254, (0+1+0)/3 = 0, (0+1+0)/3 = 0.
	if got[0].Color != "#FE0000" || got[0].ImageCount != 2 {
		t.Errorf("red bucket = %+v, want {#FE0000 2}", got[0])
	}
	// Green bucket: only image a → 1 image, single member → exact color.
	if got[1].Color != "#00FF00" || got[1].ImageCount != 1 {
		t.Errorf("green bucket = %+v, want {#00FF00 1}", got[1])
	}
}

// TestTopDominantColorsLimitAndTieBreak verifies the limit is respected and
// that equal-count buckets are ordered by color for deterministic output.
func TestTopDominantColorsLimitAndTieBreak(t *testing.T) {
	_, teardown := setupRecoveryTestDB(t, "top-colors-limit", false)
	defer teardown()

	seedImages(t, []models.Image{
		{Filename: "src/red.jpg", Type: "image", DominantColors: mustPalette(t, "#FF0000")},
		{Filename: "src/green.jpg", Type: "image", DominantColors: mustPalette(t, "#00FF00")},
		{Filename: "src/blue.jpg", Type: "image", DominantColors: mustPalette(t, "#0000FF")},
	})

	got, err := TopDominantColors(2)
	if err != nil {
		t.Fatalf("TopDominantColors(2) failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries for limit=2, got %d", len(got))
	}
	// All buckets count 1 image → tie broken by ascending color.
	if got[0].Color != "#0000FF" || got[1].Color != "#00FF00" {
		t.Errorf("limit=2 order = [%s %s], want [#0000FF #00FF00]", got[0].Color, got[1].Color)
	}

	// Non-positive limit falls back to the default of 16 (all 3 buckets).
	got, err = TopDominantColors(0)
	if err != nil {
		t.Fatalf("TopDominantColors(0) failed: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("default limit should return all 3 buckets, got %d", len(got))
	}

	// Oversized limit is not an error; it just returns everything available.
	got, err = TopDominantColors(100)
	if err != nil {
		t.Fatalf("TopDominantColors(100) failed: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("limit=100 should return all 3 buckets, got %d", len(got))
	}
}

// TestTopDominantColorsSingleQuery proves the aggregation streams the whole
// table in one SQL statement (no per-row or chunked follow-up queries).
func TestTopDominantColorsSingleQuery(t *testing.T) {
	counter, teardown := setupRecoveryTestDB(t, "top-colors-single-query", true)
	defer teardown()

	imgs := make([]models.Image, 50)
	for i := range imgs {
		imgs[i] = models.Image{
			Filename:       fmt.Sprintf("src/q-%02d.jpg", i),
			Type:           "image",
			DominantColors: mustPalette(t, "#101010", "#F0F0F0"),
		}
	}
	seedImages(t, imgs)

	before := counter.Statements()
	got, err := TopDominantColors(16)
	if err != nil {
		t.Fatalf("TopDominantColors failed: %v", err)
	}
	if delta := counter.Statements() - before; delta != 1 {
		t.Errorf("expected exactly 1 SQL statement, got %d", delta)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 buckets, got %d", len(got))
	}
}

// TestTopDominantColorsEmptyLibrary asserts an empty library yields an empty
// (non-nil) slice so the JSON response is [] rather than null.
func TestTopDominantColorsEmptyLibrary(t *testing.T) {
	_, teardown := setupRecoveryTestDB(t, "top-colors-empty", false)
	defer teardown()

	got, err := TopDominantColors(16)
	if err != nil {
		t.Fatalf("TopDominantColors failed: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 entries for an empty library, got %d", len(got))
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}
	if string(raw) != "[]" {
		t.Errorf("expected [] JSON for an empty library, got %s", raw)
	}
}

// TestColorBucketKeyQuantizesTo16Shades pins the bucket packing (4 bits per
// channel) and proves adjacent shades merge while bucket boundaries split.
func TestColorBucketKeyQuantizesTo16Shades(t *testing.T) {
	// Packing: r=1, g=3, b=5 nibbles → 0x0135.
	if got, want := colorBucketKey(0x12, 0x34, 0x56), uint16(0x0135); got != want {
		t.Errorf("colorBucketKey(0x12,0x34,0x56) = %#x, want %#x", got, want)
	}
	// Same 4-bit bucket despite different low nibbles.
	if colorBucketKey(0xFF, 0x0A, 0x00) != colorBucketKey(0xF0, 0x0F, 0x0F) {
		t.Error("colors within one 4-bit bucket should share a key")
	}
	// One shade across a bucket boundary must split.
	if colorBucketKey(0xEF, 0x00, 0x00) == colorBucketKey(0xF0, 0x00, 0x00) {
		t.Error("colors on opposite sides of a bucket boundary should differ")
	}
}

// BenchmarkTopDominantColors measures the full-table palette aggregation
// behind GET /colors/top over 2000 images x 5 colors.
func BenchmarkTopDominantColors(b *testing.B) {
	_, teardown := setupRecoveryTestDB(b, "bench-top-colors", false)
	defer teardown()

	imgs := make([]models.Image, 2000)
	for i := range imgs {
		palette := make([]string, NumDominantColors)
		for j := range palette {
			palette[j] = fmt.Sprintf("#%02X%02X%02X",
				(i*7+j*13)%256, (i*11+j*29)%256, (i*17+j*31)%256)
		}
		raw, err := json.Marshal(palette)
		if err != nil {
			b.Fatalf("failed to marshal palette: %v", err)
		}
		imgs[i] = models.Image{
			Filename:       fmt.Sprintf("src/b-%04d.jpg", i),
			Type:           "image",
			DominantColors: string(raw),
		}
	}
	seedImages(b, imgs)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := TopDominantColors(16); err != nil {
			b.Fatalf("TopDominantColors failed: %v", err)
		}
	}
}
