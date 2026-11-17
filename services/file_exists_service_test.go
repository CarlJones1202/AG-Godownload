package services

import (
	"context"
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// sqlCounter is a gorm logger that counts executed SQL statements so tests can
// assert query-count bounds (i.e. prove a scan is chunked, not per-row).
type sqlCounter struct {
	mu    sync.Mutex
	count int
}

func (c *sqlCounter) LogMode(_ gormlogger.LogLevel) gormlogger.Interface { return c }
func (c *sqlCounter) Info(context.Context, string, ...interface{})       {}
func (c *sqlCounter) Warn(context.Context, string, ...interface{})       {}
func (c *sqlCounter) Error(context.Context, string, ...interface{})      {}

func (c *sqlCounter) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	_, _ = fc()
	c.mu.Lock()
	c.count++
	c.mu.Unlock()
}

// Statements returns how many SQL statements were executed so far.
func (c *sqlCounter) Statements() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

// setupRecoveryTestDB opens an isolated in-memory sqlite database, points
// UploadsDir at a temp directory, and returns an SQL statement counter plus a
// teardown function that restores the swapped globals. When counting is false
// the default gorm logger is used.
func setupRecoveryTestDB(t testing.TB, name string, counting bool) (*sqlCounter, func()) {
	t.Helper()

	counter := &sqlCounter{}
	gcfg := &gorm.Config{}
	if counting {
		gcfg.Logger = counter
	}
	testDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), gcfg)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatalf("failed to get test database connection: %v", err)
	}

	originalDB := database.DB
	database.DB = testDB
	originalUploads := UploadsDir
	UploadsDir = t.TempDir()

	if err := testDB.AutoMigrate(&models.Image{}, &models.Gallery{}, &models.Source{}); err != nil {
		_ = sqlDB.Close()
		database.DB = originalDB
		UploadsDir = originalUploads
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return counter, func() {
		_ = sqlDB.Close()
		database.DB = originalDB
		UploadsDir = originalUploads
	}
}

func seedImages(t testing.TB, imgs []models.Image) {
	t.Helper()
	// Pass a pointer to a slice header sharing the caller's backing array so
	// generated IDs are written back into the caller's slice.
	seed := imgs
	if err := database.DB.CreateInBatches(&seed, 500).Error; err != nil {
		t.Fatalf("failed to seed images: %v", err)
	}
}

// countImagesWhere returns the number of image rows matching the condition.
func countImagesWhere(t *testing.T, cond string, args ...interface{}) int64 {
	t.Helper()
	var n int64
	if err := database.DB.Model(&models.Image{}).Where(cond, args...).Count(&n).Error; err != nil {
		t.Fatalf("failed to count images: %v", err)
	}
	return n
}

// TestSyncFileExistsFreshInstallFastPath proves the fresh-install fast path:
// with uploads/ missing or empty, SyncFileExists must perform a single UPDATE
// (no per-row stats, no SELECTs, no per-row updates) and mark every row
// missing.
func TestSyncFileExistsFreshInstallFastPath(t *testing.T) {
	counter, teardown := setupRecoveryTestDB(t, "sync-fresh-install", true)
	defer teardown()

	seedImages(t, []models.Image{
		{Filename: "src/a.jpg", Type: "image", FileExists: true},
		{Filename: "src/b.jpg", Type: "image", FileExists: true},
		{Filename: "src/c.mp4", Type: "video", FileExists: true},
	})

	assertAllMissing := func(t *testing.T) {
		t.Helper()
		if got := countImagesWhere(t, "file_exists = ?", false); got != 3 {
			t.Errorf("expected 3 rows marked missing, got %d", got)
		}
	}

	t.Run("missing uploads dir", func(t *testing.T) {
		UploadsDir = filepath.Join(t.TempDir(), "does-not-exist")

		before := counter.Statements()
		if err := SyncFileExists(); err != nil {
			t.Fatalf("SyncFileExists failed: %v", err)
		}
		if got := counter.Statements() - before; got != 1 {
			t.Errorf("expected exactly 1 SQL statement (single UPDATE), got %d", got)
		}
		assertAllMissing(t)
	})

	t.Run("empty uploads dir", func(t *testing.T) {
		UploadsDir = t.TempDir() // exists but contains no entries
		if err := database.DB.Model(&models.Image{}).Where("1 = 1").Update("file_exists", true).Error; err != nil {
			t.Fatalf("failed to reset flags: %v", err)
		}

		before := counter.Statements()
		if err := SyncFileExists(); err != nil {
			t.Fatalf("SyncFileExists failed: %v", err)
		}
		if got := counter.Statements() - before; got != 1 {
			t.Errorf("expected exactly 1 SQL statement (single UPDATE), got %d", got)
		}
		assertAllMissing(t)
	})
}

// TestSyncFileExistsNormalPath covers the regular stat sweep: files on disk
// flip their row to true, missing files flip to false, and already-correct
// rows are left alone.
func TestSyncFileExistsNormalPath(t *testing.T) {
	_, teardown := setupRecoveryTestDB(t, "sync-normal-path", false)
	defer teardown()

	seedImages(t, []models.Image{
		{Filename: "src/present-a.jpg", Type: "image", FileExists: false}, // on disk, flag stale
		{Filename: "src/absent.jpg", Type: "image", FileExists: true},     // missing, flag stale
		{Filename: "src/present-c.jpg", Type: "image", FileExists: true},  // on disk, flag correct
	})

	for _, rel := range []string{"src/present-a.jpg", "src/present-c.jpg"} {
		full := filepath.Join(UploadsDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	if err := SyncFileExists(); err != nil {
		t.Fatalf("SyncFileExists failed: %v", err)
	}

	assertFlag := func(filename string, want bool) {
		t.Helper()
		var img models.Image
		if err := database.DB.Where("filename = ?", filename).First(&img).Error; err != nil {
			t.Fatalf("image %s not found: %v", filename, err)
		}
		if img.FileExists != want {
			t.Errorf("image %s: file_exists = %v, want %v", filename, img.FileExists, want)
		}
	}
	assertFlag("src/present-a.jpg", true)
	assertFlag("src/absent.jpg", false)
	assertFlag("src/present-c.jpg", true)
}
