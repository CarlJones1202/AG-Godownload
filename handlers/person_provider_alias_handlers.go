package handlers

import (
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func GetProviderAliases(c *gin.Context) {
	personID := c.Param("id")

	var person models.Person
	if err := database.DB.First(&person, personID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	var aliases []models.PersonProviderAlias
	if err := database.DB.Where("person_id = ?", personID).Order("provider ASC, alias ASC").Find(&aliases).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch provider aliases"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": aliases})
}

func CreateProviderAlias(c *gin.Context) {
	personID := c.Param("id")

	var req struct {
		Provider string `json:"provider" binding:"required"`
		Alias    string `json:"alias" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var person models.Person
	if err := database.DB.First(&person, personID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	alias := models.PersonProviderAlias{
		PersonID: person.ID,
		Provider: strings.ToLower(req.Provider),
		Alias:    strings.ToLower(req.Alias),
	}

	if err := database.DB.Create(&alias).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create provider alias"})
		return
	}

	// Trigger a scan for this person and provider
	services.AddToScanQueue(person.ID, alias.Provider, alias.Alias)

	c.JSON(http.StatusCreated, alias)
}

func DeleteProviderAlias(c *gin.Context) {
	personID := c.Param("id")
	aliasID := c.Param("aliasId")

	var person models.Person
	if err := database.DB.First(&person, personID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	var alias models.PersonProviderAlias
	if err := database.DB.First(&alias, aliasID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider alias not found"})
		return
	}

	if alias.PersonID != person.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Alias does not belong to this person"})
		return
	}

	if err := database.DB.Delete(&alias).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete provider alias"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Provider alias deleted successfully"})
}
