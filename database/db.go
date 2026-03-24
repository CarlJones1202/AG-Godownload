package database

import (
	"gallery_api/logger"
	"gallery_api/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Connect(dbPath string) {
	var err error
	// Use glebarez/sqlite for pure Go implementation
	// Enable WAL mode for better concurrency and set busy timeout
	DB, err = gorm.Open(sqlite.Open(dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"), &gorm.Config{})
	if err != nil {
		logger.Fatal("Failed to connect to database:", err)
	}

	logger.Info("Database connected successfully")
}

func Migrate() {
	err := DB.AutoMigrate(
		&models.Source{},
		&models.Gallery{},
		&models.Image{},
		&models.Person{},
		&models.PersonIdentifier{},
		&models.PersonExclusion{},
		&models.PersonProviderAlias{},
		&models.PersonScanQueue{},
		&models.ScanResultExclusion{},
		&models.Tag{},
	)
	if err != nil {
		logger.Fatal("Failed to migrate database:", err)
	}
	logger.Info("Database migrated successfully")

	addPersonGalleriesIndex()
}

func addPersonGalleriesIndex() {
	err := DB.Exec(`
		CREATE INDEX IF NOT EXISTS idx_person_galleries_person_gallery 
		ON person_galleries(person_id, gallery_id)
	`).Error
	if err != nil {
		logger.Warn("Failed to add person_galleries index:", err)
		return
	}
	logger.Info("Added composite index on person_galleries(person_id, gallery_id)")

	addImageIndexes()
}

func addImageIndexes() {
	err := DB.Exec(`
		CREATE INDEX IF NOT EXISTS idx_images_type_deleted_created 
		ON images(type, deleted_at, created_at)
	`).Error
	if err != nil {
		logger.Warn("Failed to add images index:", err)
		return
	}
	logger.Info("Added composite index on images(type, deleted_at, created_at)")
}

func MigrateData() {
	// Migrate existing one-to-many relationships to many-to-many
	// We use raw SQL for performance and simplicity
	err := DB.Exec(`
		INSERT INTO image_galleries (image_id, gallery_id)
		SELECT id, gallery_id FROM images 
		WHERE gallery_id != 0 
		AND id NOT IN (SELECT image_id FROM image_galleries)
	`).Error
	if err != nil {
		logger.Warn("Data migration failed (might be already done):", err)
		return
	}
	logger.Info("Data migration (images -> image_galleries) completed")
}
