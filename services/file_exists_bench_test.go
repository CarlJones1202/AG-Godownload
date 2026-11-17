package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gallery_api/database"
	"gallery_api/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupSyncBench creates an isolated sqlite database with n image rows and
// points UploadsDir at a temp directory. It returns a cleanup function.
func setupSyncBench(b *testing.B, n int, withFiles bool) (teardown func()) {
	b.Helper()

	testDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:sync-bench-%s?mode=memory&cache=shared", b.Name())), &gorm.Config{})
	if err != nil {
		b.Fatalf("failed to open test database: %v", err)
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		b.Fatalf("failed to get test database connection: %v", err)
	}

	originalDB := database.DB
	database.DB = testDB

	originalUploads := UploadsDir
	UploadsDir = b.TempDir()

	if err := testDB.AutoMigrate(&models.Image{}); err != nil {
		b.Fatalf("failed to migrate test database: %v", err)
	}

	rows := make([]models.Image, 0, n)
	for i := 0; i < n; i++ {
		img := models.Image{
			Filename:   filepath.Join("bench-source", fmt.Sprintf("img-%06d.jpg", i)),
			Type:       "image",
			FileExists: true, // simulate a restored DB that believes files are present
		}
		if withFiles && i%2 == 0 {
			full := filepath.Join(UploadsDir, img.Filename)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				b.Fatalf("failed to create bench dir: %v", err)
			}
			if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
				b.Fatalf("failed to create bench file: %v", err)
			}
		}
		rows = append(rows, img)
	}
	if err := testDB.CreateInBatches(rows, 500).Error; err != nil {
		b.Fatalf("failed to seed bench rows: %v", err)
	}

	return func() {
		sqlDB.Close()
		database.DB = originalDB
		UploadsDir = originalUploads
	}
}

// BenchmarkSyncFileExistsFreshInstall measures the startup disk-state sync when
// uploads/ is empty (fresh install on a new machine).
func BenchmarkSyncFileExistsFreshInstall(b *testing.B) {
	teardown := setupSyncBench(b, 2000, false)
	defer teardown()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset flags so each iteration pays the full cost of a startup sync.
		if err := database.DB.Model(&models.Image{}).Where("1 = 1").Update("file_exists", true).Error; err != nil {
			b.Fatalf("failed to reset flags: %v", err)
		}
		if err := SyncFileExists(); err != nil {
			b.Fatalf("SyncFileExists failed: %v", err)
		}
	}
}

// BenchmarkSyncFileExistsPopulated measures the startup disk-state sync when
// half the expected files are present on disk (normal restart).
func BenchmarkSyncFileExistsPopulated(b *testing.B) {
	teardown := setupSyncBench(b, 2000, true)
	defer teardown()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := database.DB.Model(&models.Image{}).Where("1 = 1").Update("file_exists", true).Error; err != nil {
			b.Fatalf("failed to reset flags: %v", err)
		}
		if err := SyncFileExists(); err != nil {
			b.Fatalf("SyncFileExists failed: %v", err)
		}
	}
}

// serialFileExistsReference is a reference implementation of the pre-change
// serial stat sweep (one os.Stat per row, batched updates), kept so the
// parallel sweep can be compared against it under identical conditions.
func serialFileExistsReference() error {
	const pageSize = 500
	var lastID uint
	for {
		var page []struct {
			ID         uint
			Filename   string
			FileExists bool
		}
		if err := database.DB.Model(&models.Image{}).
			Select("id, filename, file_exists").
			Where("id > ?", lastID).
			Order("id ASC").
			Limit(pageSize).
			Find(&page).Error; err != nil {
			return err
		}
		if len(page) == 0 {
			break
		}

		var toTrueIDs, toFalseIDs []uint
		for _, img := range page {
			_, err := os.Stat(filepath.Join(UploadsDir, strings.TrimSpace(img.Filename)))
			exists := err == nil
			if exists != img.FileExists {
				if exists {
					toTrueIDs = append(toTrueIDs, img.ID)
				} else {
					toFalseIDs = append(toFalseIDs, img.ID)
				}
			}
		}
		if len(toTrueIDs) > 0 {
			if err := database.DB.Model(&models.Image{}).Where("id IN ?", toTrueIDs).Update("file_exists", true).Error; err != nil {
				return err
			}
		}
		if len(toFalseIDs) > 0 {
			if err := database.DB.Model(&models.Image{}).Where("id IN ?", toFalseIDs).Update("file_exists", false).Error; err != nil {
				return err
			}
		}
		lastID = page[len(page)-1].ID
	}
	return nil
}

// BenchmarkSyncFileExistsSerialReferenceFreshInstall measures the reference
// serial sweep with an empty uploads dir (the pre-change fresh-install path).
func BenchmarkSyncFileExistsSerialReferenceFreshInstall(b *testing.B) {
	teardown := setupSyncBench(b, 2000, false)
	defer teardown()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := database.DB.Model(&models.Image{}).Where("1 = 1").Update("file_exists", true).Error; err != nil {
			b.Fatalf("failed to reset flags: %v", err)
		}
		if err := serialFileExistsReference(); err != nil {
			b.Fatalf("serial reference failed: %v", err)
		}
	}
}

// BenchmarkSyncFileExistsSerialReferencePopulated measures the reference
// serial sweep with half the files on disk (the pre-change normal path).
func BenchmarkSyncFileExistsSerialReferencePopulated(b *testing.B) {
	teardown := setupSyncBench(b, 2000, true)
	defer teardown()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := database.DB.Model(&models.Image{}).Where("1 = 1").Update("file_exists", true).Error; err != nil {
			b.Fatalf("failed to reset flags: %v", err)
		}
		if err := serialFileExistsReference(); err != nil {
			b.Fatalf("serial reference failed: %v", err)
		}
	}
}
