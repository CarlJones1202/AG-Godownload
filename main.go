package main

import (
	"gallery_api/database"
	"gallery_api/handlers"
	"gallery_api/logger"
	"gallery_api/services"

	"github.com/gin-gonic/gin"
)

func main() {
	// Initialize Database
	database.Connect()
	database.Migrate()
	database.MigrateData()

	// Ensure uploads directory exists
	if err := services.EnsureUploadsDir(); err != nil {
		logger.Fatal("Failed to create uploads directory:", err)
	}

	// // Migrate images to new directory structure
	// if err := services.MigrateImagesToNewStructure(); err != nil {
	// 	logger.Warn("Image migration had errors:", err)
	// }

	// Run startup verification
	go func() {
		logger.Info("Starting background verification of downloaded images...")
		if err := services.VerifyDownloadedImages(); err != nil {
			logger.Error("Background verification failed:", err)
		} else {
			logger.Info("Background verification completed successfully")
		}
	}()

	// Start background crawler worker
	services.StartCrawlerWorker()
	logger.Info("Background crawler worker started")

	r := gin.Default()

	// Routes
	r.POST("/sources", handlers.CreateSource)
	r.GET("/sources", handlers.GetSources)
	r.POST("/sources/:id/crawl", handlers.CrawlSource)
	r.DELETE("/sources/:id", handlers.DeleteSource)

	r.POST("/galleries", handlers.CreateGallery)
	r.GET("/galleries", handlers.GetGalleries)
	r.GET("/galleries/:id", handlers.GetGallery)
	r.POST("/galleries/:id/images", handlers.AddImageToGallery)
	r.DELETE("/galleries/:id", handlers.DeleteGallery)

	r.POST("/people", handlers.CreatePerson)
	r.GET("/people", handlers.GetPeople)
	r.GET("/people/:id", handlers.GetPerson)
	r.PUT("/people/:id", handlers.UpdatePerson)
	r.DELETE("/people/:id", handlers.DeletePerson)
	r.POST("/people/:id/link-galleries", handlers.LinkPersonToGalleries)
	r.DELETE("/people/:id/galleries/:galleryId", handlers.UnlinkGalleryFromPerson)

	r.GET("/stashdb/search", handlers.SearchStashDB)
	r.POST("/people/:id/stashdb/link", handlers.LinkStashDB)

	r.DELETE("/images/:id", handlers.DeleteImage)
	r.GET("/images", handlers.GetImages)

	// Static file serving
	r.GET("/images/*filepath", handlers.ServeImage)
	// r.GET("/thumbnails/:filename", handlers.ServeThumbnail) // Deprecated
	r.Static("/person-images", "./uploads/person_images")

	logger.Info("Server starting on :8080")
	r.Run(":8080")
}
