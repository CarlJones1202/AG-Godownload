package handlers

import (
	"encoding/json"
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

func SearchStashDB(c *gin.Context) {
	name := c.Query("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name parameter is required"})
		return
	}

	service := services.NewStashDBService()
	performers, err := service.SearchPerformers(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to search StashDB: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": performers})
}

func LinkStashDB(c *gin.Context) {
	personID := c.Param("id")
	var req struct {
		StashID string `json:"stash_id"`
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

	service := services.NewStashDBService()
	performer, err := service.GetPerformer(req.StashID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get performer from StashDB: " + err.Error()})
		return
	}

	// Update person with StashDB data
	// Merge aliases
	var currentAliases []string
	json.Unmarshal([]byte(person.Aliases), &currentAliases)

	// Add new aliases, avoiding duplicates
	aliasMap := make(map[string]bool)
	for _, a := range currentAliases {
		aliasMap[a] = true
	}
	for _, a := range performer.Aliases {
		if !aliasMap[a] {
			currentAliases = append(currentAliases, a)
			aliasMap[a] = true
		}
	}

	newAliasesJSON, _ := json.Marshal(currentAliases)

	// Update fields
	person.StashID = performer.ID
	person.Aliases = string(newAliasesJSON)
	// Optionally update name if you want to sync with StashDB
	// person.Name = performer.Name

	if err := database.DB.Save(&person).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update person"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Person linked to StashDB successfully", "person": person})
}
