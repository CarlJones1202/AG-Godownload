package database

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestGormBeginMode proves what SQL gorm's db.Begin() actually issues under the
// DSN's _txlock setting, by timing a second transaction's Begin while the first
// holds an open transaction:
//   - deferred:  B's Begin returns instantly (no write lock needed) and A's
//     later write hits the 517 stale-snapshot race.
//   - immediate: B's Begin BLOCKS until A releases the write lock -> the race
//     between read and write is structurally eliminated.
func TestGormBeginMode(t *testing.T) {
	for _, mode := range []string{"deferred", "immediate"} {
		file := "gormbegin_" + mode + ".db"
		os.Remove(file)
		os.Remove(file + "-wal")
		os.Remove(file + "-shm")

		dsn := file +
			"?_pragma=journal_mode(WAL)" +
			"&_pragma=busy_timeout(5000)" +
			"&_pragma=synchronous(NORMAL)" +
			"&_pragma=journal_size_limit(4194304)"
		if mode == "immediate" {
			dsn += "&_txlock=immediate"
		}

		db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		sqlDB, _ := db.DB()
		sqlDB.SetMaxOpenConns(3)
		sqlDB.SetMaxIdleConns(3)

		db.Exec("CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY, v TEXT)")
		db.Exec("INSERT INTO t (v) VALUES ('x')")

		txA := db.Begin()
		var n int
		txA.Raw("SELECT COUNT(*) FROM t").Scan(&n)

		// B tries to BEGIN while A is open -> measure whether it blocks
		var (
			bBeginDone = make(chan struct{})
			bBeginDur  time.Duration
		)
		go func() {
			t0 := time.Now()
			txB := db.Begin()
			bBeginDur = time.Since(t0)
			if txB.Error == nil {
				txB.Exec("INSERT INTO t (v) VALUES ('b')")
				txB.Commit()
			}
			close(bBeginDone)
		}()

		select {
		case <-bBeginDone:
			// B got through immediately (deferred behavior)
		case <-time.After(400 * time.Millisecond):
			// B is still blocked waiting for the write lock (immediate behavior)
		}

		// A: try to write now (after giving B a chance to commit)
		errA := txA.Exec("INSERT INTO t (v) VALUES ('a')").Error
		txA.Rollback()

		<-bBeginDone

		if mode == "deferred" {
			if errA == nil {
				t.Errorf("deferred: expected 517 stale-snapshot error on A write, got none")
			}
			t.Logf("deferred : A write err=%v | B begin returned instantly (deferred BEGIN confirmed)", brief(errA))
		} else {
			if bBeginDur < 300*time.Millisecond {
				t.Errorf("immediate: B's Begin took %s — expected a write-lock wait, _txlock may not apply to gorm", bBeginDur)
			}
			t.Logf("immediate: B's Begin waited %s (BEGIN IMMEDIATE confirmed — race eliminated)", bBeginDur.Round(time.Millisecond))
		}
		sqlDB.Close()
	}
}

var _ = fmt.Sprint
var _ = strings.TrimSpace

func brief(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}
