package database

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestBusyTimeoutWorks holds a write transaction open on conn A for 2s while
// conn B tries to BEGIN IMMEDIATE. If busy_timeout works, B blocks ~2s and
// succeeds. If the busy handler is broken, B fails instantly with SQLITE_BUSY.
func TestBusyTimeoutWorks(t *testing.T) {
	if os.Getenv("TEST_SQLITE_PROBES") != "1" {
		t.Skip("sqlite-specific probe; set TEST_SQLITE_PROBES=1 to run")
	}
	dsn := "lock_probe_busy.db" +
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

	db.Exec("DROP TABLE IF EXISTS lock_probe_busy")
	db.Exec("CREATE TABLE lock_probe_busy (id INTEGER PRIMARY KEY, v TEXT)")

	var wg sync.WaitGroup
	var (
		mu      sync.Mutex
		bErr    error
		dur     time.Duration
		commitA error
	)
	startB := make(chan struct{})

	// A: holds write lock for 2 seconds
	wg.Add(1)
	go func() {
		defer wg.Done()
		txA := db.Begin()
		if txA.Error == nil {
			txA.Exec("INSERT INTO lock_probe_busy (id, v) VALUES (1, 'a')")
			close(startB) // tell B to go
			time.Sleep(2 * time.Second)
			mu.Lock()
			commitA = txA.Commit().Error
			mu.Unlock()
		} else {
			close(startB)
		}
	}()

	// B: tries to write while A holds the lock
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-startB
		t0 := time.Now()
		txB := db.Begin()
		bErr = txB.Error
		if txB.Error == nil {
			txB.Exec("INSERT INTO lock_probe_busy (id, v) VALUES (2, 'b')")
			txB.Commit()
		}
		mu.Lock()
		dur = time.Since(t0)
		mu.Unlock()
	}()

	wg.Wait()
	t.Logf("A commit err=%v, B begin err=%v, B wait duration=%s", commitA, bErr, dur)

	if bErr != nil && strings.Contains(bErr.Error(), "locked") {
		t.Fatalf("B failed with database is locked (busy_timeout broken): %v", bErr)
	}
	if dur < 1500*time.Millisecond {
		t.Fatalf("B did not wait for the lock (took %s) — busy handler not honoring busy_timeout", dur)
	}
}
