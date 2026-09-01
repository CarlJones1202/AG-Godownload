package handlers

import (
	"gallery_api/database"
	"gallery_api/models"
	"gallery_api/services"
	"net/http"
	"strconv"
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

	// Include providers that only exist in scan history (e.g. ad-hoc scans
	// via the person page's Search) but have no explicit alias row yet, so the
	// full set of linked providers is always visible and unlinkable. IDs are
	// synthetic negatives to avoid colliding with real alias rows.
	seen := make(map[string]bool)
	for _, al := range aliases {
		seen[strings.ToLower(al.Provider)] = true
	}
	var scans []models.PersonScanQueue
	if err := database.DB.Select("DISTINCT provider, alias").Where("person_id = ?", personID).Find(&scans).Error; err == nil {
		for _, s := range scans {
			if s.Provider == "" || seen[strings.ToLower(s.Provider)] {
				continue
			}
			seen[strings.ToLower(s.Provider)] = true
			aliases = append(aliases, models.PersonProviderAlias{
				PersonID: person.ID,
				Provider: s.Provider,
				Alias:    s.Alias,
			})
		}
	}

	c.JSON(http.StatusOK, aliases)
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

	var person models.Person
	if err := database.DB.First(&person, personID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Person not found"})
		return
	}

	// Accept either a numeric alias id (real aliases) or a provider name
	// (scan-only providers that have no alias row yet).
	target := c.Param("aliasId")
	provider := ""
	if id, err := strconv.ParseUint(target, 10, 64); err == nil {
		var aliasDB models.PersonProviderAlias
		if err := database.DB.First(&aliasDB, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Provider alias not found"})
			return
		}
		if aliasDB.PersonID != person.ID {
			c.JSON(http.StatusForbidden, gin.H{"error": "Alias does not belong to this person"})
			return
		}
		provider = aliasDB.Provider
		if err := database.DB.Delete(&aliasDB).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete provider alias"})
			return
		}
	} else {
		provider = target
	}

	// Full cleanup: remove the provider's scan history, pending scans, and
	// scan-result exclusions so stale missing-gallery results stop showing up
	// once the link is removed.
	if err := database.DB.Where("person_id = ? AND provider = ?", person.ID, provider).
		Delete(&models.PersonScanQueue{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove scan history"})
		return
	}

	if err := database.DB.Where("person_id = ? AND provider = ?", person.ID, provider).
		Delete(&models.ScanResultExclusion{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove scan exclusions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Provider alias deleted successfully"})
}
