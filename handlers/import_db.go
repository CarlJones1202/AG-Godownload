package handlers

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"gallery_api/config"
	"gallery_api/logger"
	"github.com/gin-gonic/gin"
)

// ImportDB restores a PostgreSQL plain-format dump (as produced by /export/db)
// by piping it through psql with ON_ERROR_STOP. Access is gated by the
// X-Maintenance-Token header. The dump's own DROP/CREATE statements replace the
// matching tables in the target database, so this is a full restore, not a merge.
// Uploaded as multipart form field "file".
func ImportDB(c *gin.Context) {
	token := c.GetHeader("X-Maintenance-Token")
	if config.Global.MaintenanceToken != "" && strings.TrimSpace(token) != strings.TrimSpace(config.Global.MaintenanceToken) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	psql := findPgBin("psql")
	if psql == "" {
		logger.Warn("Import: psql not found (set PGBIN in .env)")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "psql not found; set PGBIN in .env"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "upload field 'file' required: " + err.Error()})
		return
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "gallery_restore_*.sql")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create temp file"})
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save upload"})
		return
	}
	if err := tmp.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to finalize upload"})
		return
	}

	start := time.Now()
	cmd := exec.Command(psql, "-d", config.Global.DatabaseDSN(), "-v", "ON_ERROR_STOP=1", "-f", tmpPath)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+config.Global.DBPassword)
	out := &strings.Builder{}
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Run(); err != nil {
		logger.Errorf("Import: psql failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "database restore failed",
			"detail": truncate(out.String(), 4096),
		})
		return
	}

	logger.Infof("Import: restored %s (%d bytes) in %s", header.Filename, header.Size, time.Since(start).Round(time.Millisecond))
	c.JSON(http.StatusOK, gin.H{
		"message":     "database restored",
		"file":        header.Filename,
		"bytes":       header.Size,
		"duration_ms": time.Since(start).Milliseconds(),
		"psql_output": truncate(out.String(), 4096),
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}