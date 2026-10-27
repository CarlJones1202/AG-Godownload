package handlers

import (
	"encoding/json"
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestGetImagesRandomSortRespectsSeed(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open("file:random-image-seed?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatalf("failed to get test database connection: %v", err)
	}
	defer sqlDB.Close()

	originalDB := database.DB
	database.DB = testDB
	defer func() { database.DB = originalDB }()

	if err := testDB.AutoMigrate(&models.Image{}, &models.Gallery{}, &models.Source{}, &models.Person{}, &models.Tag{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	images := make([]models.Image, 30)
	for i := range images {
		images[i] = models.Image{Filename: fmt.Sprintf("image-%d.jpg", i), Type: "image"}
	}
	if err := testDB.Create(&images).Error; err != nil {
		t.Fatalf("failed to create test images: %v", err)
	}

	getImageIDs := func(seed string) []uint {
		t.Helper()
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest("GET", "/images?sort_by=random&random_seed="+seed+"&limit=30", nil)

		GetImages(context)

		if recorder.Code != 200 {
			t.Fatalf("handler status = %d, want 200: %s", recorder.Code, recorder.Body.String())
		}
		var response struct {
			Data []models.Image `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("failed to decode handler response: %v", err)
		}
		ids := make([]uint, len(response.Data))
		for i, image := range response.Data {
			ids[i] = image.ID
		}
		return ids
	}

	firstOrder := getImageIDs("17")
	repeatedOrder := getImageIDs("17")
	otherSeedOrder := getImageIDs("29")
	if !reflect.DeepEqual(firstOrder, repeatedOrder) {
		t.Fatal("same random seed returned different image orders")
	}
	if reflect.DeepEqual(firstOrder, otherSeedOrder) {
		t.Fatal("different random seeds returned the same image order")
	}
}
