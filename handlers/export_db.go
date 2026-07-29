package handlers

import (
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/logger"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
)

func ExportDB(c *gin.Context) {
	if err := database.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; err != nil {
		logger.Warnf("Export: WAL truncate checkpoint failed: %v", err)
		if err2 := database.DB.Exec("PRAGMA wal_checkpoint(PASSIVE)").Error; err2 != nil {
			logger.Warnf("Export: WAL passive checkpoint also failed: %v", err2)
		}
	}

	dbPath := config.Global.DatabasePath
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve database path"})
		return
	}

	src, err := os.Open(absPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open database file"})
		return
	}
	defer src.Close()

	tmpFile, err := os.CreateTemp("", "gallery_export_*.db")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create temp file"})
		return
	}
	tmpPath := tmpFile.Name()

	if _, err := io.Copy(tmpFile, src); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to copy database"})
		return
	}
	tmpFile.Close()

	defer os.Remove(tmpPath)

	timestamp := time.Now().Format("20060102_150405")
	filename := "gallery_export_" + timestamp + ".db"

	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.File(tmpPath)
}
