package database

import (
	"gallery_api/logger"
	"gallery_api/models"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
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
			SlowThreshold:             time.Hour,
			LogLevel:                  gormlogger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
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
		Logger:                                   NewLogger(),
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
	if err := migrateDuplicateTags(); err != nil {
		logger.Fatal("Failed to migrate duplicate tags:", err)
	}
	if err := migrateDuplicateImageEmbeddings(); err != nil {
		logger.Fatal("Failed to migrate duplicate image embeddings:", err)
	}

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
		&models.RecommendationEvent{},
		&models.EmbedQueue{},
	)
	if err != nil {
		logger.Fatal("Failed to migrate database:", err)
	}
	logger.Info("Database migrated successfully")

	if err := migratePlayboyProviderAliases(); err != nil {
		logger.Fatal("Failed to migrate Playboy provider aliases:", err)
	}

	addPersonGalleriesIndex()
	addPersonScanQueueIndexes()
}

func migrateDuplicateTags() error {
	if !DB.Migrator().HasTable(&models.Tag{}) {
		return nil
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var duplicateNames []struct {
			Name string
		}
		if err := tx.Unscoped().Model(&models.Tag{}).
			Select("name").
			Group("name").
			Having("COUNT(*) > 1").
			Find(&duplicateNames).Error; err != nil {
			return err
		}

		for _, duplicate := range duplicateNames {
			var tags []models.Tag
			if err := tx.Unscoped().Where("name = ?", duplicate.Name).
				Order("CASE WHEN deleted_at IS NULL THEN 0 ELSE 1 END, id ASC").
				Find(&tags).Error; err != nil {
				return err
			}
			if len(tags) < 2 {
				continue
			}

			keepID := tags[0].ID
			for _, tag := range tags[1:] {
				if tx.Migrator().HasTable("image_tags") {
					if err := tx.Exec(`
						INSERT INTO image_tags (image_id, tag_id)
						SELECT source.image_id, ?
						FROM image_tags AS source
						WHERE source.tag_id = ?
						  AND NOT EXISTS (
							SELECT 1 FROM image_tags AS existing
							WHERE existing.image_id = source.image_id
							  AND existing.tag_id = ?
						  )
					`, keepID, tag.ID, keepID).Error; err != nil {
						return err
					}
					if err := tx.Exec("DELETE FROM image_tags WHERE tag_id = ?", tag.ID).Error; err != nil {
						return err
					}
				}

				if err := tx.Unscoped().Where("id = ?", tag.ID).Delete(&models.Tag{}).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})
}

func migrateDuplicateImageEmbeddings() error {
	if !DB.Migrator().HasTable(&models.ImageEmbedding{}) {
		return nil
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var duplicateImageIDs []uint
		if err := tx.Model(&models.ImageEmbedding{}).
			Select("image_id").
			Group("image_id").
			Having("COUNT(*) > 1").
			Pluck("image_id", &duplicateImageIDs).Error; err != nil {
			return err
		}

		for _, imageID := range duplicateImageIDs {
			var keepID uint
			if err := tx.Model(&models.ImageEmbedding{}).
				Where("image_id = ?", imageID).
				Order("created_at DESC, id DESC").
				Limit(1).
				Pluck("id", &keepID).Error; err != nil {
				return err
			}
			if err := tx.Where("image_id = ? AND id <> ?", imageID, keepID).
				Delete(&models.ImageEmbedding{}).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func migratePlayboyProviderAliases() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var aliases []models.PersonProviderAlias
		if err := tx.Where("LOWER(provider) = ?", "playboy").Find(&aliases).Error; err != nil {
			return err
		}

		personIDs := make(map[uint]bool)
		for _, alias := range aliases {
			personIDs[alias.PersonID] = true
		}

		for personID := range personIDs {
			var playboyPlusCount int64
			if err := tx.Model(&models.PersonProviderAlias{}).
				Where("person_id = ? AND LOWER(provider) = ?", personID, "playboyplus").
				Count(&playboyPlusCount).Error; err != nil {
				return err
			}

			if playboyPlusCount > 0 {
				if err := tx.Where("person_id = ? AND LOWER(provider) = ?", personID, "playboy").
					Delete(&models.PersonProviderAlias{}).Error; err != nil {
					return err
				}
				continue
			}

			if err := tx.Model(&models.PersonProviderAlias{}).
				Where("person_id = ? AND LOWER(provider) = ?", personID, "playboy").
				Update("provider", "playboyplus").Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// addPersonScanQueueIndexes helps the dashboard "missing galleries" attention
// count pick the latest completed scan per (person, provider) quickly.
func addPersonScanQueueIndexes() {
	err := DB.Exec(`
		CREATE INDEX IF NOT EXISTS idx_person_scan_queue_person_provider_id
		ON person_scan_queues(person_id, provider, id DESC)
	`).Error
	if err != nil {
		logger.Warn("Failed to add person_scan_queue index:", err)
		return
	}
	logger.Info("Added composite index on person_scan_queue(person_id, provider, id DESC)")
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
