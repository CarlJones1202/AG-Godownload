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

func TestGetImagesPersonFilterIncludesGalleryAndDirectVideos(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open("file:person-filter-videos?mode=memory&cache=shared"), &gorm.Config{})
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

	person := models.Person{Name: "Test Person"}
	if err := testDB.Create(&person).Error; err != nil {
		t.Fatalf("failed to create person: %v", err)
	}
	gallery := models.Gallery{Name: "Test Gallery"}
	if err := testDB.Create(&gallery).Error; err != nil {
		t.Fatalf("failed to create gallery: %v", err)
	}
	if err := testDB.Model(&person).Association("Galleries").Append(&gallery); err != nil {
		t.Fatalf("failed to link gallery to person: %v", err)
	}

	galleryVideo := models.Image{Filename: "gallery-video.mp4", Type: "video", Galleries: []*models.Gallery{&gallery}}
	directVideo := models.Image{Filename: "direct-video.mp4", Type: "video"}
	unrelatedVideo := models.Image{Filename: "unrelated-video.mp4", Type: "video"}
	photo := models.Image{Filename: "direct-photo.jpg", Type: "image"}
	media := []models.Image{galleryVideo, directVideo, unrelatedVideo, photo}
	if err := testDB.Create(&media).Error; err != nil {
		t.Fatalf("failed to create media: %v", err)
	}
	if err := testDB.Model(&person).Association("Images").Append(&media[3]); err != nil {
		t.Fatalf("failed to link photo to person: %v", err)
	}
	linkRecorder := httptest.NewRecorder()
	linkContext, _ := gin.CreateTestContext(linkRecorder)
	linkContext.Params = gin.Params{
		{Key: "id", Value: fmt.Sprint(person.ID)},
		{Key: "imageId", Value: fmt.Sprint(media[1].ID)},
	}
	linkContext.Request = httptest.NewRequest("POST", fmt.Sprintf("/people/%d/images/%d", person.ID, media[1].ID), nil)
	LinkImageToPerson(linkContext)
	if linkRecorder.Code != 200 {
		t.Fatalf("manual video tag status = %d, want 200: %s", linkRecorder.Code, linkRecorder.Body.String())
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("GET", fmt.Sprintf("/images?type=video&filter_person=%d&limit=10", person.ID), nil)

	GetImages(context)

	if recorder.Code != 200 {
		t.Fatalf("handler status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data []models.Image `json:"data"`
		Meta struct {
			TotalItems int `json:"total_items"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode handler response: %v", err)
	}
	if len(response.Data) != 2 || response.Meta.TotalItems != 2 {
		t.Fatalf("filtered videos = %d (total %d), want 2", len(response.Data), response.Meta.TotalItems)
	}
}
