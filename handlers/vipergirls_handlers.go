package handlers

import (
	"gallery_api/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

// SearchVipergirls returns parsed vipergirls.to search results for the popup.
func SearchVipergirls(c *gin.Context) {
	results, err := services.SearchVipergirls(c.Query("query"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, results)
}

// ScrapeVipergirls links a chosen thread to a person's missing gallery and queues it.
func ScrapeVipergirls(c *gin.Context) {
	var req services.ScrapeVipergirlsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !services.IsVipergirlsThreadURL(req.ThreadURL) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vipergirls thread URL"})
		return
	}

	response, err := services.ScrapeVipergirlsAsMissingGallery(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, response)
}
