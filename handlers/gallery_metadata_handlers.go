package handlers

import (
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

// SearchGalleryMetadata searches for matching galleries from MetArt/Playboy
func SearchGalleryMetadata(c *gin.Context) {
	id := c.Param("id")

	var gallery models.Gallery
	if err := database.DB.Preload("People").First(&gallery, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gallery not found"})
		return
	}

	// Build list of people names
	var peopleNames []string
	for _, person := range gallery.People {
		peopleNames = append(peopleNames, person.Name)
	}

	// Search for matching galleries
	results, err := services.SearchGalleryMatches(gallery.Name, peopleNames)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"count":   len(results),
	})
}

// ScrapeGalleryMetadataRequest represents the request body for scraping
type ScrapeGalleryMetadataRequest struct {
	SourceURL string `json:"source_url" binding:"required"`
	Provider  string `json:"provider" binding:"required"`
	SourceID  string `json:"source_id"` // Optional, used for API-based sources like MetArt
}

// ScrapeGalleryMetadata scrapes metadata from a confirmed gallery URL
func ScrapeGalleryMetadata(c *gin.Context) {
	id := c.Param("id")

	var req ScrapeGalleryMetadataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var gallery models.Gallery
	if err := database.DB.First(&gallery, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Gallery not found"})
		return
	}

	// Scrape metadata from the confirmed URL/ID
	metadata, err := services.ScrapeGalleryMetadata(req.SourceURL, req.Provider, req.SourceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update gallery with scraped metadata
	gallery.Provider = metadata.Provider
	gallery.Description = metadata.Description
	gallery.Rating = metadata.Rating
	gallery.ReleaseDate = metadata.ReleaseDate

	if err := database.DB.Save(&gallery).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update gallery"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Gallery metadata updated successfully",
		"gallery":  gallery,
		"metadata": metadata,
	})
}
