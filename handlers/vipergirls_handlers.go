package handlers

import (
	"gallery_api/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

// SearchVipergirls returns parsed forum search results for the popup. The endpoint
// name is retained for existing callers; results from all supported sites are combined.
func SearchVipergirls(c *gin.Context) {
	results, err := services.SearchForumThreadsWithContext(c.Request.Context(), c.Query("query"))
	if err != nil {
		if results == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "results": results})
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
	if !services.IsForumThreadURL(req.ThreadURL) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported forum thread URL"})
		return
	}

	response, err := services.ScrapeVipergirlsAsMissingGallery(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, response)
}
