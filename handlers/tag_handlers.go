package handlers

import (
	"gallery_api/database"
	"gallery_api/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetTags returns all tags with optional filtering
func GetTags(c *gin.Context) {
	category := c.Query("category") // filter by category (label, pose, mood, manual)
	limit := c.DefaultQuery("limit", "100")

	limitInt, _ := strconv.Atoi(limit)

	var tags []models.Tag
	query := database.DB.Model(&models.Tag{})

	if category != "" {
		query = query.Where("category = ?", category)
	}

	if err := query.Limit(limitInt).Order("name ASC").Find(&tags).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tags"})
		return
	}

	c.JSON(http.StatusOK, tags)
}

// GetTopTags returns the most used tags
func GetTopTags(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "16")
	category := c.Query("category")

	limit, _ := strconv.Atoi(limitStr)

	type TagWithCount struct {
		models.Tag
		ImageCount int64 `json:"image_count"`
	}

	var tags []TagWithCount

	query := database.DB.Model(&models.Tag{}).
		Select("tags.*, COUNT(image_tags.image_id) as image_count").
		Joins("LEFT JOIN image_tags ON image_tags.tag_id = tags.id").
		Group("tags.id")

	if category != "" {
		query = query.Where("tags.category = ?", category)
	}

	if err := query.Order("image_count DESC").Limit(limit).Find(&tags).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch top tags"})
		return
	}

	c.JSON(http.StatusOK, tags)
}

// CreateTag creates a new tag
func CreateTag(c *gin.Context) {
	var input struct {
		Name     string `json:"name" binding:"required"`
		Category string `json:"category"` // defaults to "manual"
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.Category == "" {
		input.Category = "manual"
	}

	tag := models.Tag{
		Name:     input.Name,
		Category: input.Category,
	}

	if err := database.DB.Create(&tag).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create tag"})
		return
	}

	c.JSON(http.StatusCreated, tag)
}

// LinkTagToImage links a tag to an image
func LinkTagToImage(c *gin.Context) {
	imageID := c.Param("imageId")
	tagID := c.Param("tagId")

	var image models.Image
	if err := database.DB.First(&image, imageID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
		return
	}

	var tag models.Tag
	if err := database.DB.First(&tag, tagID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tag not found"})
		return
	}

	if err := database.DB.Model(&image).Association("Tags").Append(&tag); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to link tag"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tag linked successfully"})
}

// UnlinkTagFromImage removes a tag from an image
func UnlinkTagFromImage(c *gin.Context) {
	imageID := c.Param("imageId")
	tagID := c.Param("tagId")

	var image models.Image
	if err := database.DB.First(&image, imageID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
		return
	}

	var tag models.Tag
	if err := database.DB.First(&tag, tagID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Tag not found"})
		return
	}

	if err := database.DB.Model(&image).Association("Tags").Delete(&tag); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to unlink tag"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tag unlinked successfully"})
}

// SearchTags searches for tags by name
func SearchTags(c *gin.Context) {
	query := c.Query("q")
	category := c.Query("category")
	limitStr := c.DefaultQuery("limit", "20")

	limit, _ := strconv.Atoi(limitStr)

	var tags []models.Tag
	dbQuery := database.DB.Model(&models.Tag{}).Where("name LIKE ?", "%"+query+"%")

	if category != "" {
		dbQuery = dbQuery.Where("category = ?", category)
	}

	if err := dbQuery.Limit(limit).Order("name ASC").Find(&tags).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to search tags"})
		return
	}

	c.JSON(http.StatusOK, tags)
}
