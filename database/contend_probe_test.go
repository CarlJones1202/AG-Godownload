package database

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestHighContention reproduces the app's concurrent write mix:
//   - write-only transactions (crawler flush / verification flush)
//   - read-then-write transactions (embed worker markEmbedProcessing)
//   - single-statement writes (EnqueueEmbed, tag links)
func TestHighContention(t *testing.T) {
	if os.Getenv("TEST_SQLITE_PROBES") != "1" {
		t.Skip("sqlite-specific probe; set TEST_SQLITE_PROBES=1 to run")
	}
	dsn := "lock_probe_contend.db" +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_txlock=immediate"

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(3)
	defer sqlDB.Close()

	db.Exec("DROP TABLE IF EXISTS lock_probe_contend")
	db.Exec("CREATE TABLE lock_probe_contend (id INTEGER PRIMARY KEY, image_id INTEGER, status TEXT, v TEXT)")
	for i := 1; i <= 50; i++ {
		db.Exec("INSERT INTO lock_probe_contend (id, image_id, status) VALUES (?, ?, 'pending')", i, i)
	}

	var busy int32
	record := func(op string, err error) {
		if err != nil && strings.Contains(err.Error(), "locked") {
			atomic.AddInt32(&busy, 1)
			t.Logf("SQLITE_BUSY in %s: %v", op, err)
		}
	}

	var wg sync.WaitGroup

	// 2 embed-worker-style goroutines: read-then-write tx per image
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 150; i++ {
				imageID := uint(i%50 + 1)
				err := db.Transaction(func(tx *gorm.DB) error {
					var q struct {
						ID     uint
						Status string
					}
					if err := tx.Raw("SELECT id, status FROM lock_probe_contend WHERE image_id = ?", imageID).Scan(&q).Error; err != nil {
						return err
					}
					time.Sleep(time.Millisecond)
					if q.ID == 0 {
						return tx.Exec("INSERT INTO lock_probe_contend (image_id, status) VALUES (?, 'processing')", imageID).Error
					}
					return tx.Exec("UPDATE lock_probe_contend SET status = 'processing' WHERE image_id = ?", imageID).Error
				})
				record(fmt.Sprintf("worker%d", w), err)
			}
		}(w)
	}

	// 2 crawler-flush-style goroutines: write-only batch tx every iteration
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 150; i++ {
				tx := db.Begin()
				for j := 0; j < 5; j++ {
					if err := tx.Exec("UPDATE lock_probe_contend SET v = ? WHERE id = ?", fmt.Sprint(i), (i+j)%50+1).Error; err != nil {
						record("flush-update", err)
						tx.Rollback()
						break
					}
				}
				if tx.Error == nil {
					record("flush-commit", tx.Commit().Error)
				}
				time.Sleep(2 * time.Millisecond)
			}
		}(w)
	}

	// single-statement writes (EnqueueEmbed style)
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 150; i++ {
				err := db.Exec("INSERT OR IGNORE INTO lock_probe_contend (image_id, status) VALUES (?, 'pending')", i%50+1).Error
				record("enqueue", err)
			}
		}(w)
	}

	wg.Wait()
	t.Logf("total SQLITE_BUSY errors: %d", busy)
}
