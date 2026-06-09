package database

import (
	"gallery_api/logger"
	"gallery_api/models"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Connect(dbPath string) {
	var err error
	// Use glebarez/sqlite for pure Go implementation
	// Enable WAL mode for better concurrency and set busy timeout
	// synchronous=NORMAL is safe with WAL (writes are still atomic) and much faster
	// cache_size=-20000 allocates 20MB page cache (default is 2MB)
	// journal_size_limit=4194304 caps WAL file at 4MB to prevent read slowdown
	DB, err = gorm.Open(sqlite.Open(dbPath+"?_pragma=journal_mode(WAL)"+
		"&_pragma=busy_timeout(5000)"+
		"&_pragma=synchronous(NORMAL)"+
		"&_pragma=cache_size(-20000)"+
		"&_pragma=journal_size_limit(4194304)"+
		"&_pragma=temp_store(MEMORY)",
	), &gorm.Config{})
	if err != nil {
		logger.Fatal("Failed to connect to database:", err)
	}

	// Configure connection pool — SQLite only supports one writer at a time,
	// so limiting to 1 open connection eliminates all lock contention overhead.
	sqlDB, err := DB.DB()
	if err != nil {
		logger.Fatal("Failed to get underlying sql.DB:", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(time.Hour)

	logger.Info("Database connected successfully")
}

// Checkpoint forces a WAL checkpoint to keep the WAL file small.
// Call after heavy write batches to prevent read performance degradation.
func Checkpoint() {
	if err := DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; err != nil {
		logger.Warnf("WAL checkpoint failed: %v", err)
	}
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

	addJunctionIndexes()
}

func addJunctionIndexes() {
	// image_galleries: used by GetGalleries (batch counts/first image) and GetImages (gallery filter)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_image_galleries_gallery_image ON image_galleries(gallery_id, image_id)`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_image_galleries_image_gallery ON image_galleries(image_id, gallery_id)`)

	// person_images: used by GetPeople (thumbnail batch) and person stats
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_person_images_person_image ON person_images(person_id, image_id)`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_person_images_image_person ON person_images(image_id, person_id)`)

	// image_tags: used by GetImages (tag filter) and person stats (top tags)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_image_tags_image_tag ON image_tags(image_id, tag_id)`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_image_tags_tag_image ON image_tags(tag_id, image_id)`)

	// person_galleries reverse index for gallery->person lookups
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_person_galleries_gallery_person ON person_galleries(gallery_id, person_id)`)

	logger.Info("Added composite indexes for all junction tables")

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
