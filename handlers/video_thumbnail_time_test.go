package handlers

import (
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupThumbnailTimeTestDB(t *testing.T) func() {
	t.Helper()

	testDB, err := gorm.Open(sqlite.Open("file:thumbnail-time-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatalf("failed to get test database connection: %v", err)
	}

	originalDB := database.DB
	database.DB = testDB
	originalUploads := services.UploadsDir
	services.UploadsDir = t.TempDir()

	if err := testDB.AutoMigrate(&models.Image{}); err != nil {
		_ = sqlDB.Close()
		database.DB = originalDB
		services.UploadsDir = originalUploads
		t.Fatalf("failed to migrate test database: %v", err)
	}

	rows := []models.Image{
		{ID: 1, Filename: filepath.Join("src", "image.jpg"), Type: "image", FileExists: true},
		{ID: 2, Filename: filepath.Join("src", "video.mp4"), Type: "video", Duration: 10, FileExists: true},
		{ID: 3, Filename: filepath.Join("src", "missing.mp4"), Type: "video", Duration: 10, FileExists: false},
	}
	if err := testDB.Create(&rows).Error; err != nil {
		_ = sqlDB.Close()
		database.DB = originalDB
		services.UploadsDir = originalUploads
		t.Fatalf("failed to seed images: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(services.UploadsDir, "src"), 0755); err != nil {
		t.Fatalf("failed to create source directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(services.UploadsDir, "src", "video.mp4"), []byte("fake"), 0644); err != nil {
		t.Fatalf("failed to create video fixture: %v", err)
	}

	return func() {
		_ = sqlDB.Close()
		database.DB = originalDB
		services.UploadsDir = originalUploads
	}
}

func callUpdateVideoThumbnailTime(t *testing.T, id int, body string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "imageId", Value: fmt.Sprint(id)}}
	ctx.Request = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/images/%d/thumbnail-time", id), strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	UpdateVideoThumbnailTime(ctx)
	return recorder
}

func TestUpdateVideoThumbnailTimeValidation(t *testing.T) {
	teardown := setupThumbnailTimeTestDB(t)
	defer teardown()

	cases := []struct {
		name string
		id   int
		body string
		want int
	}{
		{name: "missing time", id: 2, body: `{}`, want: http.StatusBadRequest},
		{name: "negative time", id: 2, body: `{"thumbnail_time_seconds": -1}`, want: http.StatusBadRequest},
		{name: "time beyond duration", id: 2, body: `{"thumbnail_time_seconds": 11}`, want: http.StatusBadRequest},
		{name: "image is rejected", id: 1, body: `{"thumbnail_time_seconds": 2}`, want: http.StatusBadRequest},
		{name: "missing file is rejected", id: 3, body: `{"thumbnail_time_seconds": 2}`, want: http.StatusConflict},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := callUpdateVideoThumbnailTime(t, tt.id, tt.body); got.Code != tt.want {
				t.Errorf("status = %d, want %d (body: %s)", got.Code, tt.want, got.Body.String())
			}
		})
	}
}
