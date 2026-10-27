package services

import (
	"gallery_api/database"
	"gallery_api/models"
	"reflect"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestAutoTagPersonMatchesProviderAliasOnGallery(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open("file:auto-tag-provider-alias?mode=memory&cache=shared"), &gorm.Config{})
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
		&models.PersonIdentifier{},
		&models.Gallery{},
		&models.Image{},
		&models.PersonProviderAlias{},
		&models.PersonExclusion{},
	); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	person := models.Person{
		Name:    "Jane Example",
		Aliases: `["Jane Alias"]`,
	}
	if err := testDB.Create(&person).Error; err != nil {
		t.Fatalf("failed to create test person: %v", err)
	}
	if err := testDB.Create(&models.PersonProviderAlias{
		PersonID: person.ID,
		Provider: "Example Provider",
		Alias:    "Provider Stage Name",
	}).Error; err != nil {
		t.Fatalf("failed to create provider alias: %v", err)
	}
	if err := testDB.Create(&models.Gallery{Name: "Provider Stage Name Showcase"}).Error; err != nil {
		t.Fatalf("failed to create test gallery: %v", err)
	}

	result, err := AutoTagPerson(person.ID, 0.6, false)
	if err != nil {
		t.Fatalf("AutoTagPerson returned an error: %v", err)
	}
	if len(result.Suggestions) != 1 {
		t.Fatalf("got %d suggestions, want 1: %#v", len(result.Suggestions), result.Suggestions)
	}
	if result.Suggestions[0].Type != "gallery" || result.Suggestions[0].MatchedOn != "Provider Stage Name" {
		t.Fatalf("unexpected provider-alias suggestion: %#v", result.Suggestions[0])
	}
}

func TestPersonSearchTermsIncludesAllAliasSources(t *testing.T) {
	person := models.Person{
		Name:    "Jane Example",
		Aliases: `["Jane Alias", "Jane Example"]`,
	}
	providerAliases := []models.PersonProviderAlias{
		{Alias: "Nordica"},
		{Alias: "  "},
	}

	got := PersonSearchTerms(person, providerAliases, "Nordica")
	want := []string{"Jane Example", "Jane Alias", "Nordica"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PersonSearchTerms() = %#v, want %#v", got, want)
	}
}
