package database

import (
	"testing"

	"gallery_api/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMigratePlayboyProviderAliases(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open("file:playboy-alias-migration?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatalf("get test database connection: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := testDB.AutoMigrate(&models.PersonProviderAlias{}); err != nil {
		t.Fatalf("migrate test schema: %v", err)
	}

	originalDB := DB
	DB = testDB
	defer func() {
		DB = originalDB
		_ = sqlDB.Close()
	}()

	aliases := []models.PersonProviderAlias{
		{PersonID: 1, Provider: "playboy", Alias: "legacy one"},
		{PersonID: 1, Provider: "PLAYBOY", Alias: "legacy two"},
		{PersonID: 2, Provider: "playboy", Alias: "redundant"},
		{PersonID: 2, Provider: "playboyplus", Alias: "existing"},
	}
	if err := testDB.Create(&aliases).Error; err != nil {
		t.Fatalf("create test aliases: %v", err)
	}

	if err := migratePlayboyProviderAliases(); err != nil {
		t.Fatalf("migrate Playboy aliases: %v", err)
	}

	var activeAliases []models.PersonProviderAlias
	if err := testDB.Find(&activeAliases).Error; err != nil {
		t.Fatalf("query active aliases: %v", err)
	}
	if len(activeAliases) != 3 {
		t.Fatalf("active alias count = %d, want 3", len(activeAliases))
	}

	personOneCount := 0
	personTwoCount := 0
	for _, alias := range activeAliases {
		switch alias.PersonID {
		case 1:
			personOneCount++
			if alias.Provider != "playboyplus" {
				t.Errorf("person 1 alias provider = %q, want playboyplus", alias.Provider)
			}
		case 2:
			personTwoCount++
			if alias.Provider != "playboyplus" || alias.Alias != "existing" {
				t.Errorf("unexpected retained person 2 alias: %#v", alias)
			}
		}
	}
	if personOneCount != 2 {
		t.Errorf("person 1 alias count = %d, want both legacy aliases migrated", personOneCount)
	}
	if personTwoCount != 1 {
		t.Errorf("person 2 alias count = %d, want existing PlayboyPlus alias only", personTwoCount)
	}

	if err := migratePlayboyProviderAliases(); err != nil {
		t.Fatalf("second migration run: %v", err)
	}
}
