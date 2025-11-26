package main

import (
	"gallery_api/database"
	"gallery_api/handlers"
	"gallery_api/services"
	"log"

	"github.com/gin-gonic/gin"
)

func main() {
	// Initialize Database
	database.Connect()
	database.Migrate()
	database.MigrateData()

	// Ensure uploads directory exists
	if err := services.EnsureUploadsDir(); err != nil {
		log.Fatal("Failed to create uploads directory:", err)
	}

	// Run startup verification
	services.VerifyDownloadedImages()

	// Start background crawler worker
	services.StartCrawlerWorker()
	log.Println("Background crawler worker started")

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

	r.DELETE("/images/:id", handlers.DeleteImage)
	r.GET("/images", handlers.GetImages)

	// Static file serving
	r.GET("/images/:filename", handlers.ServeImage)
	r.GET("/thumbnails/:filename", handlers.ServeThumbnail)

	log.Println("Server starting on :8080")
	r.Run(":8080")
}
