package services

import (
	"fmt"
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/models"
	"os"
	"path/filepath"
	"testing"
)

// TestCollectMissingImageTasks covers Phase 1 collection semantics: only
// non-video rows with file_exists = false are missing, videos are excluded
// entirely (the video pipeline owns them), and rows without a download URL are
// counted as missing but produce no task.
func TestCollectMissingImageTasks(t *testing.T) {
	_, teardown := setupRecoveryTestDB(t, "collect-images", false)
	defer teardown()

	seedImages(t, []models.Image{
		{Filename: "a/1.jpg", Type: "image", FileExists: false, DownloadURL: "http://example.com/1.jpg"}, // task
		{Filename: "a/2.jpg", Type: "image", FileExists: false, DownloadURL: ""},                         // missing, no task
		{Filename: "v/3.mp4", Type: "video", FileExists: false, DownloadURL: "http://example.com/3.mp4"}, // excluded
		{Filename: "a/4.jpg", Type: "image", FileExists: true, DownloadURL: "http://example.com/4.jpg"},  // present
		{Filename: "a/5.jpg", Type: "", FileExists: false, DownloadURL: "http://example.com/5.jpg"},      // legacy type, task
	})

	tasks, missing, err := collectMissingImageTasks()
	if err != nil {
		t.Fatalf("collectMissingImageTasks failed: %v", err)
	}
	if missing != 3 {
		t.Errorf("missing = %d, want 3 (video row must not be counted)", missing)
	}
	if len(tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(tasks))
	}

	byURL := make(map[string]ImageRecoveryTask, len(tasks))
	for _, task := range tasks {
		byURL[task.DownloadURL] = task
	}
	if _, ok := byURL["http://example.com/1.jpg"]; !ok {
		t.Error("expected task for image 1")
	}
	if _, ok := byURL["http://example.com/5.jpg"]; !ok {
		t.Error("expected task for legacy-typed image 5")
	}
	for _, task := range tasks {
		if task.SourceName != "uncategorized" {
			t.Errorf("task %d: SourceName = %q, want uncategorized", task.ID, task.SourceName)
		}
	}
}

// TestCollectMissingImageTasksQueryCount bounds the collection to chunked
// queries: ceil(rows/chunk)+1 statements regardless of row count — no per-row
// stats and no per-row UPDATEs.
func TestCollectMissingImageTasksQueryCount(t *testing.T) {
	counter, teardown := setupRecoveryTestDB(t, "collect-images-queries", true)
	defer teardown()

	const rows = 2500 // 1000-row chunks: 3 full + 1 empty
	imgs := make([]models.Image, rows)
	for i := range imgs {
		imgs[i] = models.Image{
			Filename:   fmt.Sprintf("bulk/img-%05d.jpg", i),
			Type:       "image",
			FileExists: false,
		}
	}
	seedImages(t, imgs)

	before := counter.Statements()
	tasks, missing, err := collectMissingImageTasks()
	used := counter.Statements() - before

	if err != nil {
		t.Fatalf("collectMissingImageTasks failed: %v", err)
	}
	if missing != rows {
		t.Errorf("missing = %d, want %d", missing, rows)
	}
	if len(tasks) != 0 {
		t.Errorf("got %d tasks, want 0 (no download URLs)", len(tasks))
	}
	maxQueries := (rows+999)/1000 + 1
	if used > maxQueries {
		t.Errorf("expected <= %d queries for %d rows, got %d", maxQueries, rows, used)
	}
}

// TestCollectMissingVideoTasks covers video collection semantics: batched
// source naming (direct source_id wins over the linked gallery), skip rules
// for local-only and URL-less videos, and exclusion of present rows.
func TestCollectMissingVideoTasks(t *testing.T) {
	counter, teardown := setupRecoveryTestDB(t, "collect-videos", true)
	defer teardown()

	direct := models.Source{Name: "direct-source", Location: "test://direct"}
	if err := database.DB.Create(&direct).Error; err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	viaGallery := models.Source{Name: "gallery-source", Location: "test://gallery"}
	if err := database.DB.Create(&viaGallery).Error; err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	gal := models.Gallery{Name: "g1", SourceID: &viaGallery.ID}
	if err := database.DB.Create(&gal).Error; err != nil {
		t.Fatalf("failed to create gallery: %v", err)
	}

	videos := []models.Image{
		{Filename: "v/1.mp4", Type: "video", DownloadURL: "http://example.com/1.mp4", SourceID: &direct.ID},                     // direct
		{Filename: "v/2.mp4", Type: "video", DownloadURL: "http://example.com/2.mp4", SourceID: &direct.ID},                     // direct wins
		{Filename: "v/3.mp4", Type: "video", DownloadURL: "http://example.com/3.mp4"},                                           // gallery fallback
		{Filename: "v/4.mp4", Type: "video", DownloadURL: "http://example.com/4.mp4"},                                           // uncategorized
		{Filename: "v/5.mp4", Type: "video", DownloadURL: "http://example.com/5.mp4", OriginalURL: "file:///no/such/local.mp4"}, // skipped: local file
		{Filename: "v/6.mp4", Type: "video", DownloadURL: ""},                                                                   // skipped: no download URL
		{Filename: "v/7.mp4", Type: "video", DownloadURL: "http://example.com/7.mp4", FileExists: true},                         // present: excluded
	}
	seedImages(t, videos)
	if videos[1].ID == 0 || videos[2].ID == 0 {
		t.Fatal("seed did not assign IDs")
	}
	// Video 2 has a direct source AND a gallery link (direct must win);
	// video 3 only has the gallery link (fallback).
	if err := database.DB.Model(&videos[1]).Association("Galleries").Append(&gal); err != nil {
		t.Fatalf("failed to link video 2 to gallery: %v", err)
	}
	if err := database.DB.Model(&videos[2]).Association("Galleries").Append(&gal); err != nil {
		t.Fatalf("failed to link video 3 to gallery: %v", err)
	}

	before := counter.Statements()
	tasks, missing, skipped, err := collectMissingVideoTasks()
	used := counter.Statements() - before

	if err != nil {
		t.Fatalf("collectMissingVideoTasks failed: %v", err)
	}
	if missing != 6 {
		t.Errorf("missing = %d, want 6 (present row excluded, skipped rows still missing)", missing)
	}
	if skipped != 2 {
		t.Errorf("skipped = %d, want 2 (file:// row and row without download URL)", skipped)
	}
	if len(tasks) != 4 {
		t.Fatalf("got %d tasks, want 4", len(tasks))
	}

	byName := make(map[string]string, len(tasks))
	for _, task := range tasks {
		byName[task.CurrentDBPath] = task.SourceName
	}
	want := map[string]string{
		"v/1.mp4": "direct-source",
		"v/2.mp4": "direct-source",
		"v/3.mp4": "gallery-source",
		"v/4.mp4": "uncategorized",
	}
	for filename, sourceName := range want {
		if got := byName[filename]; got != sourceName {
			t.Errorf("%s: SourceName = %q, want %q", filename, got, sourceName)
		}
	}

	// ceil(rows/500) collection chunks (incl. the empty end-of-keyset probe)
	// + 1 batched source-name lookup.
	maxQueries := (len(videos)+499)/500 + 3
	if used > maxQueries {
		t.Errorf("expected <= %d queries for %d rows, got %d", maxQueries, len(videos), used)
	}
}

// TestCollectMissingVideoTasksQueryCount bounds collection to chunked queries
// (ceil(rows/500)+3) regardless of row count — no per-row source queries.
func TestCollectMissingVideoTasksQueryCount(t *testing.T) {
	counter, teardown := setupRecoveryTestDB(t, "collect-videos-queries", true)
	defer teardown()

	src := models.Source{Name: "bulk-source"}
	if err := database.DB.Create(&src).Error; err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	const rows = 1200 // 500-row chunks: 3 full + 1 empty, + 1 source-name lookup
	videos := make([]models.Image, rows)
	for i := range videos {
		videos[i] = models.Image{
			Filename:    fmt.Sprintf("bulk/v-%04d.mp4", i),
			Type:        "video",
			DownloadURL: "http://example.com/v.mp4",
			SourceID:    &src.ID,
		}
	}
	seedImages(t, videos)

	before := counter.Statements()
	tasks, missing, skipped, err := collectMissingVideoTasks()
	used := counter.Statements() - before

	if err != nil {
		t.Fatalf("collectMissingVideoTasks failed: %v", err)
	}
	if missing != rows || len(tasks) != rows || skipped != 0 {
		t.Errorf("missing = %d, tasks = %d, skipped = %d; want %d, %d, 0", missing, len(tasks), skipped, rows, rows)
	}
	maxQueries := (rows+499)/500 + 3
	if used > maxQueries {
		t.Errorf("expected <= %d queries for %d rows, got %d", maxQueries, rows, used)
	}
}

// TestResolveSourceNamesBatchDirectWins proves the direct source_id link wins
// over the gallery's source, gallery fallback works, and unresolvable IDs are
// absent from the result.
func TestResolveSourceNamesBatchDirectWins(t *testing.T) {
	_, teardown := setupRecoveryTestDB(t, "resolve-source-names", false)
	defer teardown()

	direct := models.Source{Name: "direct-source", Location: "test://direct"}
	viaGallery := models.Source{Name: "gallery-source", Location: "test://gallery"}
	if err := database.DB.Create(&direct).Error; err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	if err := database.DB.Create(&viaGallery).Error; err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	gal := models.Gallery{Name: "g", SourceID: &viaGallery.ID}
	if err := database.DB.Create(&gal).Error; err != nil {
		t.Fatalf("failed to create gallery: %v", err)
	}

	imgs := []models.Image{
		{Filename: "1.jpg", Type: "image", SourceID: &direct.ID}, // direct + gallery
		{Filename: "2.jpg", Type: "image"},                       // gallery only
		{Filename: "3.jpg", Type: "image"},                       // neither
		{Filename: "4.jpg", Type: "image", SourceID: &direct.ID}, // direct only
	}
	seedImages(t, imgs)
	if err := database.DB.Model(&imgs[0]).Association("Galleries").Append(&gal); err != nil {
		t.Fatalf("failed to link image 1: %v", err)
	}
	if err := database.DB.Model(&imgs[1]).Association("Galleries").Append(&gal); err != nil {
		t.Fatalf("failed to link image 2: %v", err)
	}

	ids := make([]uint, len(imgs))
	for i, img := range imgs {
		ids[i] = img.ID
	}
	names := resolveSourceNamesBatch(database.DB, ids)

	if got := names[imgs[0].ID]; got != "direct-source" {
		t.Errorf("image 1: %q, want direct-source (direct link must win)", got)
	}
	if got := names[imgs[1].ID]; got != "gallery-source" {
		t.Errorf("image 2: %q, want gallery-source", got)
	}
	if _, ok := names[imgs[2].ID]; ok {
		t.Errorf("image 3: unexpected name %q", names[imgs[2].ID])
	}
	if got := names[imgs[3].ID]; got != "direct-source" {
		t.Errorf("image 4: %q, want direct-source", got)
	}
}

// TestBuildUploadsFileIndex covers the lazily-built basename -> path index
// that replaced the per-video recursive walk: nested files are found, the
// first walk-order match wins for duplicate basenames, and a missing
// directory yields an empty index instead of an error.
func TestBuildUploadsFileIndex(t *testing.T) {
	_, teardown := setupRecoveryTestDB(t, "uploads-index", false)
	defer teardown()

	UploadsDir = filepath.Join(t.TempDir(), "missing")
	if idx := buildUploadsFileIndex(); len(idx) != 0 {
		t.Errorf("expected empty index for missing uploads dir, got %d entries", len(idx))
	}

	UploadsDir = t.TempDir()
	files := []string{
		filepath.Join("alpha", "one.jpg"),
		filepath.Join("beta", "two.jpg"),
		"root.jpg",
		filepath.Join("alpha", "dup.jpg"),
		filepath.Join("beta", "dup.jpg"),
	}
	for _, rel := range files {
		full := filepath.Join(UploadsDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	idx := buildUploadsFileIndex()
	if len(idx) != 4 {
		t.Fatalf("got %d index entries, want 4 (dup basenames collapse)", len(idx))
	}
	for base, want := range map[string]string{
		"one.jpg":  filepath.Join(UploadsDir, "alpha", "one.jpg"),
		"two.jpg":  filepath.Join(UploadsDir, "beta", "two.jpg"),
		"root.jpg": filepath.Join(UploadsDir, "root.jpg"),
		// filepath.Walk visits lexical order: alpha/ before beta/.
		"dup.jpg": filepath.Join(UploadsDir, "alpha", "dup.jpg"),
	} {
		if got := idx[base]; got != want {
			t.Errorf("idx[%q] = %q, want %q", base, got, want)
		}
	}
}

// TestRecoveryConfigFallbacks proves the pool-size helpers fall back to the
// documented defaults (10 / 4) when config was not loaded, and otherwise use
// the configured values.
func TestRecoveryConfigFallbacks(t *testing.T) {
	origConcurrency := config.Global.RecoveryConcurrency
	origPost := config.Global.RecoveryPostprocessWorkers
	defer func() {
		config.Global.RecoveryConcurrency = origConcurrency
		config.Global.RecoveryPostprocessWorkers = origPost
	}()

	config.Global.RecoveryConcurrency = 0
	if got := recoveryConcurrency(); got != 10 {
		t.Errorf("recoveryConcurrency() = %d, want fallback 10", got)
	}
	config.Global.RecoveryConcurrency = 7
	if got := recoveryConcurrency(); got != 7 {
		t.Errorf("recoveryConcurrency() = %d, want 7", got)
	}

	config.Global.RecoveryPostprocessWorkers = 0
	if got := recoveryPostprocessWorkers(); got != 4 {
		t.Errorf("recoveryPostprocessWorkers() = %d, want fallback 4", got)
	}
	config.Global.RecoveryPostprocessWorkers = 3
	if got := recoveryPostprocessWorkers(); got != 3 {
		t.Errorf("recoveryPostprocessWorkers() = %d, want 3", got)
	}
}
