package main

import (
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/services"
)

func main() {
	config.Load()
	logger.SetLevelFromString(config.Global.LogLevel)
	database.Connect(config.Global.DatabasePath)
	database.Migrate()
	// Run cleanup
	if err := services.CleanupVideoGalleries(); err != nil {
		logger.Errorf("CleanupVideoGalleries failed: %v", err)
	}
}
