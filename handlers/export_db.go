package handlers

import (
	"bytes"
	"gallery_api/config"
	"gallery_api/logger"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
)

// ExportDB streams a logical dump of the PostgreSQL database produced by
// pg_dump. The dump command is located under config.Global.PGBinDir (set by
// init.ps1) or on PATH.
func ExportDB(c *gin.Context) {
	pgDump := ""
	if config.Global.PGBinDir != "" {
		candidate := filepath.Join(config.Global.PGBinDir, "pg_dump")
		if _, err := os.Stat(candidate); err == nil {
			pgDump = candidate
		} else if _, err := os.Stat(candidate + ".exe"); err == nil {
			pgDump = candidate + ".exe"
		}
	}
	if pgDump == "" {
		if p, err := exec.LookPath("pg_dump"); err == nil {
			pgDump = p
		}
	}
	if pgDump == "" {
		logger.Warn("Export: pg_dump not found (set PGBIN in .env)")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "pg_dump not found; set PGBIN in .env"})
		return
	}

	dsn := config.Global.DatabaseDSN()
	tmpPath := ""
	{
		tmp, err := os.CreateTemp("", "gallery_export_*.sql")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create temp file"})
			return
		}
		tmpPath = tmp.Name()
		tmp.Close()
	}
	defer os.Remove(tmpPath)

	cmd := exec.Command(pgDump, "--no-owner", "--no-privileges", "--dbname", dsn)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+config.Global.DBPassword)

	outFile, err := os.Create(tmpPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create output file"})
		return
	}
	cmd.Stdout = outFile
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		outFile.Close()
		logger.Warnf("Export: pg_dump failed: %v (%s)", err, stderr.String())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database dump failed"})
		return
	}
	outFile.Close()

	timestamp := time.Now().Format("20060102_150405")
	filename := "gallery_export_" + timestamp + ".sql"

	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.File(tmpPath)
}
