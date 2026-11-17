package database

import (
	"testing"

	"gallery_api/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMigrateDuplicateTags(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open("file:duplicate-tags-migration?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatalf("get test database connection: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()

	if err := testDB.Exec(`
		CREATE TABLE tags (
			id INTEGER PRIMARY KEY,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			name TEXT,
			category TEXT
		)
	`).Error; err != nil {
		t.Fatalf("create tags table: %v", err)
	}
	if err := testDB.Exec(`
		CREATE TABLE image_tags (
			image_id INTEGER NOT NULL,
			tag_id INTEGER NOT NULL,
			PRIMARY KEY (image_id, tag_id)
		)
	`).Error; err != nil {
		t.Fatalf("create image_tags table: %v", err)
	}
	if err := testDB.Exec(`
		INSERT INTO tags (id, name, category, deleted_at) VALUES
			(1, 'same-name', 'legacy', CURRENT_TIMESTAMP),
			(2, 'same-name', 'manual', NULL),
			(3, 'same-name', 'label', NULL)
	`).Error; err != nil {
		t.Fatalf("insert duplicate tags: %v", err)
	}
	if err := testDB.Exec(`
		INSERT INTO image_tags (image_id, tag_id) VALUES
			(10, 1),
			(10, 2),
			(20, 3)
	`).Error; err != nil {
		t.Fatalf("insert image-tag links: %v", err)
	}

	originalDB := DB
	DB = testDB
	defer func() {
		DB = originalDB
	}()

	if err := migrateDuplicateTags(); err != nil {
		t.Fatalf("migrate duplicate tags: %v", err)
	}
	if err := testDB.Exec("CREATE UNIQUE INDEX idx_tags_name ON tags(name)").Error; err != nil {
		t.Fatalf("create unique tag index after migration: %v", err)
	}

	var tags []models.Tag
	if err := testDB.Unscoped().Find(&tags).Error; err != nil {
		t.Fatalf("query migrated tags: %v", err)
	}
	if len(tags) != 1 || tags[0].ID != 2 || tags[0].Category != "manual" || tags[0].DeletedAt.Valid {
		t.Fatalf("unexpected retained tags: %#v", tags)
	}

	var links []struct {
		ImageID uint
		TagID   uint
	}
	if err := testDB.Table("image_tags").Order("image_id").Find(&links).Error; err != nil {
		t.Fatalf("query migrated image tags: %v", err)
	}
	if len(links) != 2 ||
		links[0].ImageID != 10 || links[0].TagID != 2 ||
		links[1].ImageID != 20 || links[1].TagID != 2 {
		t.Fatalf("unexpected migrated image-tag links: %#v", links)
	}

	if err := migrateDuplicateTags(); err != nil {
		t.Fatalf("repeat duplicate-tag migration: %v", err)
	}
}

func TestMigrateDuplicateImageEmbeddings(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open("file:duplicate-image-embeddings-migration?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatalf("get test database connection: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()

	if err := testDB.Exec(`
		CREATE TABLE image_embeddings (
			id INTEGER PRIMARY KEY,
			image_id INTEGER,
			embedder TEXT,
			dimension INTEGER,
			vector BLOB,
			created_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("create image_embeddings table: %v", err)
	}
	if err := testDB.Exec(`
		INSERT INTO image_embeddings (id, image_id, embedder, created_at) VALUES
			(1, 5, 'old-model', '2026-01-01 00:00:00'),
			(2, 5, 'new-model', '2026-01-02 00:00:00'),
			(3, 5, 'new-model', '2026-01-02 00:00:00'),
			(4, 6, 'new-model', '2026-01-02 00:00:00')
	`).Error; err != nil {
		t.Fatalf("insert duplicate embeddings: %v", err)
	}

	originalDB := DB
	DB = testDB
	defer func() {
		DB = originalDB
	}()

	if err := migrateDuplicateImageEmbeddings(); err != nil {
		t.Fatalf("migrate duplicate image embeddings: %v", err)
	}
	if err := testDB.AutoMigrate(&models.ImageEmbedding{}); err != nil {
		t.Fatalf("create unique image embedding index after migration: %v", err)
	}
	var indexColumns []struct {
		Seqno int
		Cid   int
		Name  string
	}
	if err := testDB.Raw("PRAGMA index_info(idx_image_embeddings_image_id)").Scan(&indexColumns).Error; err != nil {
		t.Fatalf("inspect image embedding index: %v", err)
	}
	if len(indexColumns) != 1 || indexColumns[0].Name != "image_id" {
		t.Fatalf("unexpected image embedding index columns: %#v", indexColumns)
	}

	var embeddings []struct {
		ID       uint
		ImageID  uint
		Embedder string
	}
	if err := testDB.Table("image_embeddings").Order("image_id").Find(&embeddings).Error; err != nil {
		t.Fatalf("query migrated image embeddings: %v", err)
	}
	if len(embeddings) != 2 ||
		embeddings[0].ID != 3 || embeddings[0].ImageID != 5 || embeddings[0].Embedder != "new-model" ||
		embeddings[1].ID != 4 || embeddings[1].ImageID != 6 {
		t.Fatalf("unexpected migrated image embeddings: %#v", embeddings)
	}

	if err := migrateDuplicateImageEmbeddings(); err != nil {
		t.Fatalf("repeat duplicate image embedding migration: %v", err)
	}
}
