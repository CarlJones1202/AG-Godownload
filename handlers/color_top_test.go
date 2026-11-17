package handlers

import (
	"encoding/json"
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// topColorEntry mirrors the GetTopColors JSON response shape.
type topColorEntry struct {
	Color      string `json:"color"`
	ImageCount int    `json:"image_count"`
}

// setupTopColorsDB points database.DB at a fresh in-memory sqlite database
// seeded with seed image rows, each contributing one distinct color bucket,
// and returns a teardown function that restores the original connection.
func setupTopColorsDB(t *testing.T, name string, seed int) func() {
	t.Helper()

	testDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatalf("failed to get test database connection: %v", err)
	}

	originalDB := database.DB
	database.DB = testDB

	if err := testDB.AutoMigrate(&models.Image{}); err != nil {
		_ = sqlDB.Close()
		database.DB = originalDB
		t.Fatalf("failed to migrate test database: %v", err)
	}

	if seed > 0 {
		rows := make([]models.Image, seed)
		for i := range rows {
			// Each row lands in its own 4-bit bucket: nibble pair (i%16, i/16)
			// uniquely identifies i for i < 256.
			rows[i] = models.Image{
				Filename:       fmt.Sprintf("src/top-%03d.jpg", i),
				Type:           "image",
				DominantColors: fmt.Sprintf(`["#%02X%02X%02X"]`, (i%16)<<4|8, ((i/16)%16)<<4|8, 0x80),
			}
		}
		if err := testDB.CreateInBatches(&rows, 500).Error; err != nil {
			_ = sqlDB.Close()
			database.DB = originalDB
			t.Fatalf("failed to seed images: %v", err)
		}
	}

	return func() {
		_ = sqlDB.Close()
		database.DB = originalDB
	}
}

// callGetTopColors invokes GetTopColors with the given raw query string
// (e.g. "?limit=2") and returns the recorder plus the decoded body.
func callGetTopColors(t *testing.T, query string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/colors/top"+query, nil)
	GetTopColors(ctx)
	return recorder, recorder.Body.String()
}

// TestGetTopColorsClampsLimit verifies the default (16), low clamping
// (non-positive or unparsable → default), and the 64 upper bound, and that
// every entry has a valid #RRGGBB color and a positive image count.
func TestGetTopColorsClampsLimit(t *testing.T) {
	teardown := setupTopColorsDB(t, "get-top-colors-clamp", 70)
	defer teardown()

	cases := []struct {
		query string
		want  int
	}{
		{"", 16},           // default
		{"?limit=2", 2},    // explicit low limit
		{"?limit=999", 64}, // clamped to 64
		{"?limit=abc", 16}, // unparsable → default
		{"?limit=0", 16},   // non-positive → default
	}

	for _, tc := range cases {
		recorder, body := callGetTopColors(t, tc.query)
		if recorder.Code != 200 {
			t.Errorf("limit%q: status = %d, want 200 (body %s)", tc.query, recorder.Code, body)
			continue
		}
		var entries []topColorEntry
		if err := json.Unmarshal([]byte(body), &entries); err != nil {
			t.Errorf("limit%q: failed to decode response %q: %v", tc.query, body, err)
			continue
		}
		if len(entries) != tc.want {
			t.Errorf("limit%q: got %d entries, want %d", tc.query, len(entries), tc.want)
		}
		for _, e := range entries {
			if len(e.Color) != 7 || !strings.HasPrefix(e.Color, "#") {
				t.Errorf("limit%q: invalid color %q", tc.query, e.Color)
			}
			if e.ImageCount < 1 {
				t.Errorf("limit%q: color %s has image_count %d, want >= 1", tc.query, e.Color, e.ImageCount)
			}
		}
	}
}

// TestGetTopColorsEmptyLibrary asserts an empty library responds with a JSON
// [] array (never null) and HTTP 200.
func TestGetTopColorsEmptyLibrary(t *testing.T) {
	teardown := setupTopColorsDB(t, "get-top-colors-empty", 0)
	defer teardown()

	recorder, body := callGetTopColors(t, "")
	if recorder.Code != 200 {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if strings.TrimSpace(body) != "[]" {
		t.Errorf("body = %q, want []", body)
	}
}
