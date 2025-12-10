package handlers

import (
	"encoding/json"
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ListIdentifierSources returns all available identifier sources
func ListIdentifierSources(c *gin.Context) {
	registry := services.GetIdentifierRegistry()
	sources := registry.ListProviders()
	c.JSON(http.StatusOK, gin.H{"sources": sources})
}

// SearchIdentifier searches for people using a specific identifier source
func SearchIdentifier(c *gin.Context) {
	source := c.Param("source")
	name := c.Query("name")

	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name parameter is required"})
		return
	}

	registry := services.GetIdentifierRegistry()
	provider, err := registry.GetProvider(source)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	results, err := provider.Search(name)
	if err != nil {
		fmt.Printf("Identifier search error (%s): %v\n", source, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to search %s: %v", source, err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": results})
}

// LinkIdentifier links a person to an external identifier
func LinkIdentifier(c *gin.Context) {
	personID := c.Param("id")

	var req struct {
		Source     string `json:"source" binding:"required"`
		ExternalID string `json:"external_id" binding:"required"`
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

	// Get the identifier provider
	registry := services.GetIdentifierRegistry()
	provider, err := registry.GetProvider(req.Source)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Fetch person data from the provider
	personData, err := provider.GetDetails(req.ExternalID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get details from %s: %v", req.Source, err)})
		return
	}

	// Update person with data from identifier
	updatePersonFromData(&person, personData)

	// Create or update identifier record
	var identifier models.PersonIdentifier
	result := database.DB.Where("person_id = ? AND source = ?", person.ID, req.Source).First(&identifier)

	rawDataJSON, _ := json.Marshal(personData.RawData)

	if result.Error != nil {
		// Create new identifier
		identifier = models.PersonIdentifier{
			PersonID:   person.ID,
			Source:     req.Source,
			ExternalID: req.ExternalID,
			Data:       string(rawDataJSON),
		}
		if err := database.DB.Create(&identifier).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create identifier"})
			return
		}
	} else {
		// Update existing identifier
		identifier.ExternalID = req.ExternalID
		identifier.Data = string(rawDataJSON)
		if err := database.DB.Save(&identifier).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update identifier"})
			return
		}
	}

	// Download and store photos
	if len(personData.Photos) > 0 {
		var photoURLs []string
		for _, imgURL := range personData.Photos {
			localPath, err := services.DownloadPersonImage(imgURL, person.ID)
			if err != nil {
				fmt.Printf("Failed to download image %s: %v\n", imgURL, err)
				continue
			}
			photoURLs = append(photoURLs, localPath)
		}

		if len(photoURLs) > 0 {
			photosJSON, _ := json.Marshal(photoURLs)
			person.Photos = string(photosJSON)
		}
	}

	// Save person
	if err := database.DB.Save(&person).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update person"})
		return
	}

	// Backward compatibility: Update StashID if source is stashdb
	if req.Source == "stashdb" {
		person.StashID = req.ExternalID
		database.DB.Save(&person)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    fmt.Sprintf("Person linked to %s successfully", req.Source),
		"person":     person,
		"identifier": identifier,
	})
}

// UnlinkIdentifier removes an identifier from a person
func UnlinkIdentifier(c *gin.Context) {
	personID := c.Param("id")
	identifierID := c.Param("identifierId")

	var person models.Person
	if err := database.DB.First(&person, personID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	var identifier models.PersonIdentifier
	if err := database.DB.First(&identifier, identifierID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Identifier not found"})
		return
	}

	// Verify the identifier belongs to this person
	if identifier.PersonID != person.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Identifier does not belong to this person"})
		return
	}

	// Delete the identifier
	if err := database.DB.Delete(&identifier).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete identifier"})
		return
	}

	// Backward compatibility: Clear StashID if removing stashdb identifier
	if identifier.Source == "stashdb" {
		person.StashID = ""
		database.DB.Save(&person)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Identifier unlinked successfully"})
}

// Helper function to update person from PersonData
func updatePersonFromData(person *models.Person, data *services.PersonData) {
	// Merge aliases
	var currentAliases []string
	json.Unmarshal([]byte(person.Aliases), &currentAliases)

	aliasMap := make(map[string]bool)
	for _, a := range currentAliases {
		aliasMap[a] = true
	}
	for _, a := range data.Aliases {
		if !aliasMap[a] {
			currentAliases = append(currentAliases, a)
			aliasMap[a] = true
		}
	}

	newAliasesJSON, _ := json.Marshal(currentAliases)
	person.Aliases = string(newAliasesJSON)

	// Update other fields (only if not already set)
	if person.Birthdate == "" {
		person.Birthdate = data.Birthdate
	}
	if person.Country == "" {
		person.Country = data.Country
	}
	if person.Ethnicity == "" {
		person.Ethnicity = data.Ethnicity
	}
	if person.EyeColor == "" {
		person.EyeColor = data.EyeColor
	}
	if person.HairColor == "" {
		person.HairColor = data.HairColor
	}
	if person.Height == "" {
		person.Height = data.Height
	}
	if person.Measurements == "" {
		person.Measurements = data.Measurements
	}
	if person.Tattoos == "" {
		person.Tattoos = data.Tattoos
	}
	if person.Piercings == "" {
		person.Piercings = data.Piercings
	}
	if person.Twitter == "" {
		person.Twitter = data.Twitter
	}
	if person.Instagram == "" {
		person.Instagram = data.Instagram
	}
}
