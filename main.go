package main

import (
	"gallery_api/database"
	"gallery_api/handlers"
	"gallery_api/services"
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	// Initialize Database
	database.Connect()
	database.Migrate()

	// Ensure uploads directory exists
	if err := services.EnsureUploadsDir(); err != nil {
		log.Fatal("Failed to create uploads directory:", err)
	}

	// Start background crawler worker (checks every 5 minutes)
	services.StartCrawlerWorker(5 * time.Second)
	log.Println("Background crawler worker started")

	r := gin.Default()

	// Routes
	r.POST("/sources", handlers.CreateSource)
	r.GET("/sources", handlers.GetSources)
	r.POST("/sources/:id/crawl", handlers.CrawlSource)

	r.POST("/galleries", handlers.CreateGallery)
	r.GET("/galleries", handlers.GetGalleries)
	r.GET("/galleries/:id", handlers.GetGallery)
	r.POST("/galleries/:id/images", handlers.AddImageToGallery)

	// Static file serving
	// Note: In a real app, you might want to protect these or use a proper static file server (nginx etc)
	// But for this API, we serve them via endpoints to handle logic if needed, or just static.
	// The user asked to "serve the thumbnails and images to the client upon request".
	// Using specific handlers gives us more control (e.g. if we want to check permissions later).
	r.GET("/images/:filename", handlers.ServeImage)
	r.GET("/thumbnails/:filename", handlers.ServeThumbnail)

	log.Println("Server starting on :8080")
	r.Run(":8080")
}
