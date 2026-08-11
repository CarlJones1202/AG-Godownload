package database

import (
	"gallery_api/logger"
	"gallery_api/models"
	"time"

	gormlogger "gorm.io/gorm/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

// gormLogWriter forwards GORM's logged messages into the app logger. Only
// errors are surfaced (GORM is opened at Warn level with a 1h slow threshold),
// so slow-SQL warnings and per-query SQL lines don't flood the log.
type gormLogWriter struct{}

func (gormLogWriter) Printf(format string, v ...interface{}) {
	logger.Errorf("[gorm] "+format, v...)
}

// NewLogger returns a GORM logger that routes into the app logger and disables
// the default slow-SQL warnings, which flood sub-second queries on this
// machine. Real errors are still surfaced.
func NewLogger() gormlogger.Interface {
	return gormlogger.New(
		gormLogWriter{},
		gormlogger.Config{
			SlowThreshold:              time.Hour,
			LogLevel:                   gormlogger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:       true,
		},
	)
}

// Connect opens a GORM connection to PostgreSQL using a lib/pq-style DSN.
func Connect(dsn string) {
	var err error
	// Foreign keys are not emitted: the legacy sqlite database never enforced
	// them and rows use sentinel ids (e.g. gallery_id = 0 for "no gallery").
	// Enabling FK constraints would reject both those rows and new inserts.
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger: NewLogger(),
	})
	if err != nil {
		logger.Fatal("Failed to connect to database:", err)
	}

	// Configure the connection pool. PostgreSQL handles concurrency natively,
	// so we can be more generous than SQLite's single-writer limit.
	sqlDB, err := DB.DB()
	if err != nil {
		logger.Fatal("Failed to get underlying sql.DB:", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	logger.Info("Database connected successfully")
}

// Checkpoint is retained for callers that previously forced an SQLite WAL
// checkpoint. PostgreSQL commits are durable at COMMIT time, so this is a no-op.
func Checkpoint() {
	// no-op
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
		&models.ImageEmbedding{},
		&models.ImageRating{},
		&models.EmbedQueue{},
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
		CREATE INDEX IF NOT EXISTS idx_images_type_deleted_created_id 
		ON images(type, deleted_at, created_at, id)
	`).Error
	if err != nil {
		logger.Warn("Failed to add images index:", err)
		return
	}
	logger.Info("Added composite index idx_images_type_deleted_created_id on images(type, deleted_at, created_at, id)")

	// Covering index for gallery-thumbnail window function:
	//   PARTITION BY ig.gallery_id ORDER BY i.created_at ASC, i.id ASC
	//   WHERE i.deleted_at IS NULL
	// Leading with deleted_at lets SQLite scan pre-sorted rows into the window function.
	err = DB.Exec(`
		CREATE INDEX IF NOT EXISTS idx_images_deleted_created_id 
		ON images(deleted_at, created_at, id)
	`).Error
	if err != nil {
		logger.Warn("Failed to add images deleted/created index:", err)
		return
	}
	logger.Info("Added composite index idx_images_deleted_created_id on images(deleted_at, created_at, id)")

	err = DB.Exec(`
		CREATE INDEX IF NOT EXISTS idx_images_type_file_exists_deleted_created 
		ON images(type, file_exists, deleted_at, created_at, id)
	`).Error
	if err != nil {
		logger.Warn("Failed to add file_exists index:", err)
		return
	}
	logger.Info("Added composite index idx_images_type_file_exists_deleted_created on images")
}

// Shutdown closes the database connection. PostgreSQL persists each committed
// transaction to disk, so no explicit checkpoint is required on shutdown.
func Shutdown() {
	if DB == nil {
		return
	}
	logger.Info("Database shutdown: closing connection pool...")
	sqlDB, err := DB.DB()
	if err != nil {
		logger.Errorf("Failed to get underlying sql.DB for shutdown: %v", err)
		return
	}

	if err := sqlDB.Close(); err != nil {
		logger.Errorf("Failed to close database connection: %v", err)
	} else {
		logger.Info("Database connection closed cleanly")
	}
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
