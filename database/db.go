package database

import (
	"gallery_api/models"
	"log"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Connect() {
	var err error
	// Use glebarez/sqlite for pure Go implementation
	DB, err = gorm.Open(sqlite.Open("gallery.db"), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	log.Println("Database connected successfully")
}

func Migrate() {
	err := DB.AutoMigrate(&models.Source{}, &models.Gallery{}, &models.Image{})
	if err != nil {
		log.Fatal("Failed to migrate database:", err)
	}
	log.Println("Database migrated successfully")
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
		log.Printf("Warning: Data migration failed (might be already done): %v", err)
	} else {
		log.Println("Data migration (images -> image_galleries) completed")
	}
}
