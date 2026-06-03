package main

import (
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/handlers"
	"gallery_api/logger"
	"gallery_api/services"

	"github.com/gin-gonic/gin"
)

func main() {
	// Load Configuration
	config.Load()

	// Configure logger from config
	logger.SetLevelFromString(config.Global.LogLevel)

	// Initialize Database
	database.Connect(config.Global.DatabasePath)
	database.Migrate()
	database.MigrateData()

	// Clean up duplicate scan results from previous runs
	if err := services.CleanupDuplicateScans(); err != nil {
		logger.Warnf("Failed to cleanup duplicate scans: %v", err)
	}

	// Ensure uploads directory exists
	if err := services.EnsureUploadsDir(); err != nil {
		logger.Fatal("Failed to create uploads directory:", err)
	}

	// // Migrate images to new directory structure
	// if err := services.MigrateImagesToNewStructure(); err != nil {
	// 	logger.Warn("Image migration had errors:", err)
	// }

	// Run startup verification tasks concurrently
	go func() {
		logger.Info("Starting migration for missing provider thumbnails...")
		if err := services.MigrateMissingProviderThumbnails(); err != nil {
			logger.Error("Provider thumbnail migration failed:", err)
		}

		if valid, fixed, err := services.ValidateProviderThumbnails(); err != nil {
			logger.Error("Provider thumbnail validation failed:", err)
		} else {
			logger.Infof("Provider thumbnail validation: %d valid, %d fixed/updated", valid, fixed)
		}
	}()

	go func() {
		logger.Info("Starting background verification of downloaded images...")
		if err := services.RemoveDuplicateImages(); err != nil {
			logger.Error("Duplicate image removal failed:", err)
		}

		if err := services.VerifyDownloadedImages(); err != nil {
			logger.Error("Background verification failed:", err)
		} else {
			logger.Info("Background verification completed successfully")
		}
	}()

	go func() {
		logger.Info("Starting scanning for missing video metadata...")
		if err := services.ScanMissingMetadata(database.DB, false); err != nil {
			logger.Error("Video metadata scan failed:", err)
		} else {
			logger.Info("Video metadata scan completed successfully")
		}
	}()

	go func() {
		logger.Info("Starting background verification of person images...")
		if err := services.VerifyPersonImages(); err != nil {
			logger.Error("Person image verification failed:", err)
		}
	}()

	go func() {
		logger.Info("Starting background verification of videos...")
		if err := services.VerifyDownloadedVideos(); err != nil {
			logger.Error("Video verification failed:", err)
		} else {
			logger.Info("Video verification started successfully")
		}
	}()

	// Start workers
	services.StartCrawlerWorker()
	services.StartAITagWorker()
	services.StartWebSocketHub()
	services.StartScanWorker()
	services.StartDailyScanScheduler()
	logger.Info("Background workers started")

	r := gin.Default()

	// Routes
	r.POST("/sources", handlers.CreateSource)
	r.GET("/sources", handlers.GetSources)
	r.POST("/sources/:id/crawl", handlers.CrawlSource)
	r.DELETE("/sources/:id", handlers.DeleteSource)
	r.PATCH("/sources/:id/priority", handlers.UpdateSourcePriority)
	r.POST("/sources/bulk-import", handlers.BulkImportSources)
	r.GET("/downloads/status", handlers.GetDownloadStatus)
	r.GET("/ws", services.HandleWebSocket)
	// Maintenance: cleanup duplicate images
	r.POST("/cleanup-dupes", handlers.CleanupDupes)

	r.POST("/galleries", handlers.CreateGallery)
	r.GET("/galleries", handlers.GetGalleries)
	r.GET("/galleries/:id", handlers.GetGallery)
	r.POST("/galleries/:id/images", handlers.AddImageToGallery)
	r.DELETE("/galleries/:id", handlers.DeleteGallery)
	r.PUT("/galleries/:id", handlers.UpdateGallery)
	r.GET("/galleries/:id/search-metadata", handlers.SearchGalleryMetadata)
	r.POST("/galleries/:id/scrape-metadata", handlers.ScrapeGalleryMetadata)
	r.POST("/galleries/:id/update-provider", handlers.UpdateGalleryProvider)

	r.POST("/people", handlers.CreatePerson)
	r.GET("/people", handlers.GetPeople)
	r.GET("/people/:id", handlers.GetPerson)
	r.PUT("/people/:id", handlers.UpdatePerson)
	r.DELETE("/people/:id", handlers.DeletePerson)
	r.POST("/people/:id/link-galleries", handlers.LinkPersonToGalleries)
	r.POST("/people/:id/galleries/:galleryId", handlers.LinkGalleryToPerson)
	r.DELETE("/people/:id/galleries/:galleryId", handlers.UnlinkGalleryFromPerson)
	r.POST("/people/:id/images/:imageId", handlers.LinkImageToPerson)
	r.DELETE("/people/:id/images/:imageId", handlers.UnlinkImageFromPerson)

	// New identifier system routes
	r.GET("/identifiers/sources", handlers.ListIdentifierSources)
	r.GET("/identifiers/:source/search", handlers.SearchIdentifier)
	r.POST("/people/:id/identifiers", handlers.LinkIdentifier)
	r.DELETE("/people/:id/identifiers/:identifierId", handlers.UnlinkIdentifier)

	// Auto-tag routes
	r.POST("/people/:id/auto-tag", handlers.AutoTagPerson)
	r.POST("/people/:id/auto-tag/apply", handlers.ApplyAutoTagSuggestions)
	r.POST("/people/:id/exclude-gallery/:galleryId", handlers.ExcludeGalleryFromPerson)
	r.POST("/people/:id/exclude-video/:imageId", handlers.ExcludeVideoFromPerson)
	r.GET("/people/:id/exclusions", handlers.GetPersonExclusions)
	r.DELETE("/people/:id/exclusions/:exclusionId", handlers.RemoveExclusion)

	// Provider alias routes
	r.GET("/people/:id/provider-aliases", handlers.GetProviderAliases)
	r.POST("/people/:id/provider-aliases", handlers.CreateProviderAlias)
	r.DELETE("/people/:id/provider-aliases/:aliasId", handlers.DeleteProviderAlias)

	// Person scan routes
	r.POST("/people/:id/scan", handlers.TriggerPersonScan)
	r.GET("/people/:id/scans", handlers.GetPersonScanResults)
	r.POST("/people/:id/link-found-gallery", handlers.LinkFoundGallery)
	r.POST("/people/:id/link-unsure-gallery", handlers.LinkUnsureGallery)
	r.POST("/people/:id/exclude-scan-result", handlers.ExcludeScanResult)

	// Admin: all missing galleries across all people
	r.GET("/admin/missing-galleries", handlers.GetAllMissingGalleries)

	// Old StashDB routes (kept for backward compatibility)
	r.GET("/stashdb/search", handlers.SearchStashDB)
	r.POST("/people/:id/stashdb/link", handlers.LinkStashDB)

	// Stats routes
	r.GET("/people/:id/stats", handlers.GetPersonStats)

	// Source scan routes
	r.GET("/people/:id/scan", handlers.ScanPersonFromSource)

	// Tag routes
	r.GET("/tags", handlers.GetTags)
	r.GET("/tags/top", handlers.GetTopTags)
	r.GET("/tags/search", handlers.SearchTags)
	r.POST("/tags", handlers.CreateTag)
	r.POST("/images/:imageId/tags/:tagId", handlers.LinkTagToImage)
	r.DELETE("/images/:imageId/tags/:tagId", handlers.UnlinkTagFromImage)

	// Image routes
	r.DELETE("/images/:imageId", handlers.DeleteImage)
	r.POST("/images/:imageId/favorite", handlers.ToggleFavorite)
	r.PATCH("/images/:imageId/vr-mode", handlers.UpdateImageVrMode)
	r.GET("/images", handlers.GetImages)
	r.GET("/search/color", handlers.SearchByColor)

	// Static file serving
	r.GET("/thumbnails/*filepath", handlers.ServeThumbnail)
	r.GET("/images/*filepath", handlers.ServeImage)
	// r.GET("/thumbnails/:filename", handlers.ServeThumbnail) // Deprecated
	r.Static("/person-images", "./uploads/person_images")

	logger.Info("Server starting on :" + config.Global.Port)
	r.Run(":" + config.Global.Port)
}
