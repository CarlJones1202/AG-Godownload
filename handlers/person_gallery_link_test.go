package handlers

import (
	"encoding/json"
	"gallery_api/database"
	"gallery_api/models"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestLinkPersonToGalleriesMatchesProviderAlias(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open("file:link-person-provider-alias?mode=memory&cache=shared"), &gorm.Config{})
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

	if err := testDB.AutoMigrate(
		&models.Person{},
		&models.Gallery{},
		&models.Source{},
		&models.PersonProviderAlias{},
		&models.PersonScanQueue{},
	); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	person := models.Person{Name: "Jane Example", Aliases: `["Jane Alias"]`}
	if err := testDB.Create(&person).Error; err != nil {
		t.Fatalf("failed to create test person: %v", err)
	}
	providerAlias := models.PersonProviderAlias{
		PersonID: person.ID,
		Provider: "Example Provider",
		Alias:    "Nordica",
	}
	if err := testDB.Create(&providerAlias).Error; err != nil {
		t.Fatalf("failed to create provider alias: %v", err)
	}
	source := models.Source{
		Name:     "Nordica source",
		Type:     "url",
		Location: "https://example.com/nordica/",
	}
	if err := testDB.Create(&source).Error; err != nil {
		t.Fatalf("failed to create test source: %v", err)
	}
	gallery := models.Gallery{Name: "A gallery", SourceID: &source.ID}
	if err := testDB.Create(&gallery).Error; err != nil {
		t.Fatalf("failed to create test gallery: %v", err)
	}
	existingGallery := models.Gallery{Name: "Previously linked gallery"}
	if err := testDB.Create(&existingGallery).Error; err != nil {
		t.Fatalf("failed to create previously linked gallery: %v", err)
	}
	if err := testDB.Model(&person).Association("Galleries").Append(&existingGallery); err != nil {
		t.Fatalf("failed to create pre-existing gallery link: %v", err)
	}

	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	defer gin.SetMode(previousGinMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: "1"}}
	context.Request = httptest.NewRequest("POST", "/people/1/link-galleries", nil)

	LinkPersonToGalleries(context)

	if recorder.Code != 200 {
		t.Fatalf("handler status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		MatchedCount int `json:"matched_count"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode handler response: %v", err)
	}
	if response.MatchedCount != 1 {
		t.Fatalf("matched_count = %d, want 1", response.MatchedCount)
	}

	var linkedCount int64
	if err := testDB.Table("person_galleries").
		Where("person_id = ?", person.ID).
		Count(&linkedCount).Error; err != nil {
		t.Fatalf("failed to check gallery link: %v", err)
	}
	if linkedCount != 2 {
		t.Fatalf("gallery link count = %d, want 2 (provider-alias match plus pre-existing link)", linkedCount)
	}
	var matchedLinkCount int64
	if err := testDB.Table("person_galleries").
		Where("person_id = ? AND gallery_id = ?", person.ID, gallery.ID).
		Count(&matchedLinkCount).Error; err != nil {
		t.Fatalf("failed to check provider-alias gallery link: %v", err)
	}
	if matchedLinkCount != 1 {
		t.Fatalf("provider-alias gallery link count = %d, want 1", matchedLinkCount)
	}
}
