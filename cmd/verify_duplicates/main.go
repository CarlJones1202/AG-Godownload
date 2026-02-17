package main

import (
	"fmt"
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/services"
)

func main() {
	// Load Configuration
	config.Load()
	logger.SetLevelFromString(config.Global.LogLevel)

	// Initialize Database
	database.Connect(config.Global.DatabasePath)

	logger.Info("Starting manual duplicate verification...")

	// Run the function
	if err := services.RemoveDuplicateImages(); err != nil {
		logger.Error("Duplicate image removal failed:", err)
		return
	}

	fmt.Println("Verification script finished successfully.")
}
