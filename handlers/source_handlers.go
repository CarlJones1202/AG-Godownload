package handlers

import (
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func CreateSource(c *gin.Context) {
	var source models.Source
	if err := c.ShouldBindJSON(&source); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := database.DB.Create(&source).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create source"})
		return
	}

	// Automatically create a gallery for this source
	gallery := models.Gallery{
		Name:     source.Name,
		SourceID: &source.ID,
	}
	if err := database.DB.Create(&gallery).Error; err != nil {
		// Log error but don't fail the request? Or fail?
		// Let's fail for now as it's a requirement
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create default gallery for source"})
		return
	}

	c.JSON(http.StatusCreated, source)
}

func CrawlSource(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid source ID"})
		return
	}

	// Trigger crawl in background
	go func() {
		if err := services.CrawlSource(uint(id)); err != nil {
			// Log error (in a real app, use a logger)
			println("Crawl failed:", err.Error())
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{"message": "Crawl started"})
}

func GetSources(c *gin.Context) {
	var sources []models.Source
	if err := database.DB.Find(&sources).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch sources"})
		return
	}

	c.JSON(http.StatusOK, sources)
}
