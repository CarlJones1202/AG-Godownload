package handlers

import (
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func SuggestProviderAliases(c *gin.Context) {
	var person models.Person
	if err := database.DB.First(&person, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	if strings.TrimSpace(config.Global.TPDBAPIKey) == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "TPDB API key is not configured. Set TPDB_API_KEY in .env and restart the server."})
		return
	}

	name := strings.TrimSpace(c.Query("name"))
	if name == "" {
		name = person.Name
	}
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Person name is required"})
		return
	}

	results, err := services.NewTPDBAliasService(config.Global.TPDBAPIKey).Search(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": results})
}
