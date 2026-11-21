package handlers

import (
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/services"
	"github.com/gin-gonic/gin"
)

// ImportDB restores a PostgreSQL plain-format dump (as produced by /export/db)
// by piping it through psql with ON_ERROR_STOP. Access is gated by the
// X-Maintenance-Token header. The dump's own DROP/CREATE statements replace the
// matching tables in the target database, so this is a full restore, not a merge.
// Uploaded as multipart form field "file".
func ImportDB(c *gin.Context) {
	if !authorizeMaintenance(c) {
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

	start := time.Now()

	// Terminate other connections and reset public schema first so dumps without --clean restore into a fresh schema
	resetCmd := exec.Command(psql, "-d", config.Global.DatabaseDSN(), "-c", "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = current_database() AND pid <> pg_backend_pid(); DROP SCHEMA public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO postgres; GRANT ALL ON SCHEMA public TO public;")
	resetCmd.Env = append(os.Environ(), "PGPASSWORD="+config.Global.DBPassword)
	if resetOut, err := resetCmd.CombinedOutput(); err != nil {
		logger.Warnf("Import: schema reset warning: %v (%s)", err, string(resetOut))
	}

	cmd := exec.Command(psql, "-d", config.Global.DatabaseDSN(), "-v", "ON_ERROR_STOP=1", "-f", "-")
	cmd.Env = append(os.Environ(), "PGPASSWORD="+config.Global.DBPassword)
	cmd.Stdin = file
	out := &tailBuffer{limit: 4096}
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Run(); err != nil {
		logger.Errorf("Import: psql failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "database restore failed",
			"detail": out.String(),
		})
		return
	}

	logger.Infof("Import: restored %s (%d bytes) in %s", header.Filename, header.Size, time.Since(start).Round(time.Millisecond))

	// Reconnect to database after restore
	logger.Info("Import: reconnecting to database...")
	database.Connect(config.Global.DatabaseDSN())
	database.Migrate()
	database.MigrateData()

	if services.StartMissingFileRecovery() {
		logger.Info("Import: started missing-file recovery")
	} else {
		logger.Info("Import: missing-file recovery is already running")
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "database restored",
		"file":        header.Filename,
		"bytes":       header.Size,
		"duration_ms": time.Since(start).Milliseconds(),
		"psql_output": out.String(),
	})
}

type tailBuffer struct {
	mu    sync.Mutex
	limit int
	buf   []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.limit <= 0 {
		return len(p), nil
	}
	if len(p) >= b.limit {
		b.buf = append(b.buf[:0], p[len(p)-b.limit:]...)
		return len(p), nil
	}

	if excess := len(b.buf) + len(p) - b.limit; excess > 0 {
		copy(b.buf, b.buf[excess:])
		b.buf = b.buf[:len(b.buf)-excess]
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
