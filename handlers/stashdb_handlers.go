package handlers

import (
	"encoding/json"
	"fmt"
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"strings"

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
		fmt.Printf("StashDB Search Error: %v\n", err)
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

	// Store extended data
	person.Birthdate = performer.Birthdate.Date
	person.Country = performer.Country
	person.Ethnicity = performer.Ethnicity
	person.EyeColor = performer.EyeColor
	person.HairColor = performer.HairColor
	person.Height = fmt.Sprintf("%d", performer.Height)

	// Format measurements
	person.Measurements = fmt.Sprintf("Band: %d, Cup: %s, Waist: %d, Hip: %d",
		performer.Measurements.BandSize, performer.Measurements.CupSize,
		performer.Measurements.Waist, performer.Measurements.Hip)

	// Format tattoos
	var tattoos []string
	for _, t := range performer.Tattoos {
		tattoos = append(tattoos, fmt.Sprintf("%s (%s)", t.Description, t.Location))
	}
	person.Tattoos = strings.Join(tattoos, "; ")

	// Format piercings
	var piercings []string
	for _, p := range performer.Piercings {
		piercings = append(piercings, fmt.Sprintf("%s (%s)", p.Description, p.Location))
	}
	person.Piercings = strings.Join(piercings, "; ")

	// person.Bio = performer.Details // Removed as field is invalid

	// Extract social media
	for _, u := range performer.URLs {
		if strings.Contains(strings.ToLower(u.URL), "twitter.com") || strings.Contains(strings.ToLower(u.URL), "x.com") {
			parts := strings.Split(u.URL, "/")
			if len(parts) > 0 {
				person.Twitter = parts[len(parts)-1]
			}
		}
		if strings.Contains(strings.ToLower(u.URL), "instagram.com") {
			parts := strings.Split(u.URL, "/")
			if len(parts) > 0 {
				person.Instagram = parts[len(parts)-1]
			}
		}
	}

	// Download and store images
	var photoURLs []string
	personIDUint := person.ID

	for _, img := range performer.Images {
		localPath, err := services.DownloadPersonImage(img.URL, personIDUint)
		if err != nil {
			fmt.Printf("Failed to download image %s: %v\n", img.URL, err)
			continue
		}
		photoURLs = append(photoURLs, localPath)
	}

	if len(photoURLs) > 0 {
		photosJSON, _ := json.Marshal(photoURLs)
		person.Photos = string(photosJSON)
	}

	if err := database.DB.Save(&person).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update person"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Person linked to StashDB successfully", "person": person})
}
