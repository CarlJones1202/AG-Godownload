package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestDeferredUpgradeRace proves the bug that _txlock=immediate fixes:
// a deferred-BEGIN read-then-write transaction fails INSTANTLY with
// SQLITE_BUSY when another connection commits in between (busy_timeout does
// not apply to the shared->reserved upgrade). Returns per-mode result.
func TestDeferredUpgradeRace(t *testing.T) {
	for _, mode := range []string{"deferred", "immediate"} {
		file := "upgrade_" + mode + ".db"
		os.Remove(file)
		os.Remove(file + "-wal")
		os.Remove(file + "-shm")

		dsn := file +
			"?_pragma=journal_mode(WAL)" +
			"&_pragma=busy_timeout(3000)" +
			"&_pragma=synchronous(NORMAL)"
		if mode == "immediate" {
			dsn += "&_txlock=immediate"
		}

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

		db.Exec("CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY, v TEXT)")

		ctx := context.Background()
		connB, _ := sqlDB.Conn(ctx)
		defer connB.Close()

		// A: read-then-write transaction. deferred mode = raw BEGIN (deferred);
		// immediate mode = driver BeginTx so _txlock=immediate applies.
		var (
			execA   func(string, ...interface{}) error
			finishA func()
		)
		if mode == "deferred" {
			connA, _ := sqlDB.Conn(ctx)
			connA.ExecContext(ctx, "BEGIN")
			execA = func(q string, args ...interface{}) error {
				_, err := connA.ExecContext(ctx, q, args...)
				return err
			}
			finishA = func() { connA.ExecContext(ctx, "ROLLBACK") }
			defer connA.Close()
		} else {
			txA, _ := sqlDB.Begin()
			execA = func(q string, args ...interface{}) error {
				_, err := txA.ExecContext(ctx, q, args...)
				return err
			}
			finishA = func() { txA.Rollback() }
			defer txA.Rollback()
		}

		// A: read, establishing a snapshot
		if err := execA("SELECT COUNT(*) FROM t"); err != nil {
			t.Fatalf("A read: %v", err)
		}

		// B: commits a write in between
		connB.ExecContext(ctx, "BEGIN IMMEDIATE")
		connB.ExecContext(ctx, "INSERT INTO t (v) VALUES ('b')")
		connB.ExecContext(ctx, "COMMIT")

		// A: tries to write -> deferred mode must fail instantly, immediate mode waits
		t0 := time.Now()
		errA := execA("INSERT INTO t (v) VALUES ('a')")
		elapsed := time.Since(t0).Round(time.Millisecond)
		finishA()

		t.Logf("mode=%-9s A-write-after-B-commit: err=%v elapsed=%s", mode, errA, elapsed)
		if mode == "deferred" && errA == nil {
			t.Errorf("deferred mode should fail (that is the bug being fixed)")
		}
		if mode == "immediate" && errA != nil {
			t.Errorf("immediate mode should serialize and succeed, got: %v", errA)
		}
	}
}
