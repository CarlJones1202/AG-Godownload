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

	// Check for duplicate location
	var existing models.Source
	if err := database.DB.Where("location = ?", source.Location).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"error":           "A source with this URL already exists",
			"existing_source": existing,
		})
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
