package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestReaderLockMode proves whether sqlDB.Begin() with _txlock=immediate turns
// a READ transaction into a write-lock-holding transaction that blocks writers.
// readerStyle: "gorm" = via database/sql Begin(), "raw" = pinned conn plain BEGIN.
func TestReaderLockMode(t *testing.T) {
	for _, rs := range []string{"gorm-begin", "raw-begin"} {
		file := "readerlock_" + rs + ".db"
		os.Remove(file)
		os.Remove(file + "-wal")
		os.Remove(file + "-shm")

		dsn := file +
			"?_pragma=journal_mode(WAL)" +
			"&_pragma=busy_timeout(3000)" +
			"&_pragma=synchronous(NORMAL)" +
			"&_pragma=wal_autocheckpoint(0)" +
			"&_txlock=immediate"

		db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatalf("sql db: %v", err)
		}
		sqlDB.SetMaxOpenConns(4)
		sqlDB.SetMaxIdleConns(4)

		table := "t_" + strings.ReplaceAll(rs, "-", "_")
		db.Exec("CREATE TABLE " + table + " (id INTEGER PRIMARY KEY, v TEXT)")
		tx := db.Begin()
		for i := 0; i < 5000; i++ {
			tx.Exec("INSERT INTO "+table+" (v) VALUES (?)", strings.Repeat("x", 50))
		}
		tx.Commit()

		ctx := context.Background()
		if rs == "gorm-begin" {
			r, _ := sqlDB.Begin()
			r.Query("SELECT COUNT(*) FROM " + table)
			defer r.Rollback()
		} else {
			conn, _ := sqlDB.Conn(ctx)
			conn.ExecContext(ctx, "BEGIN")
			conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(new(int))
			defer conn.Close() // close rolls back
		}

		// single writer attempt
		t0 := time.Now()
		wt := db.Begin()
		beginErr := wt.Error
		beginDur := time.Since(t0)
		if beginErr == nil {
			wt.Exec("INSERT INTO " + table + " (v) VALUES ('probe')")
			beginErr = wt.Commit().Error
		}
		sqlDB.Close()

		status := "WRITER-OK"
		if beginErr != nil {
			status = "WRITER-BLOCKED"
		}
		t.Logf("%-10s reader -> %s (begin/commit err=%v, waited %s)", rs, status, beginErr, beginDur.Round(time.Millisecond))
	}
}
