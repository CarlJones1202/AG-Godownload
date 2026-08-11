package database

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestProdShape mirrors the production config exactly (default autocheckpoint,
// journal_size_limit 4MB, _txlock=immediate, busy_timeout) with SHORT readers
// (API browsing: begin -> query -> immediately rollback) running concurrently
// with small writers. Measures commit latency; any commit > busy_timeout is a
// production BUSY error.
func TestProdShape(t *testing.T) {
	if os.Getenv("TEST_SQLITE_PROBES") != "1" {
		t.Skip("sqlite-specific probe; set TEST_SQLITE_PROBES=1 to run")
	}
	type run struct {
		name       string
		extraPrag  string
		ckp        bool
		readerKeep time.Duration
	}
	runs := []run{
		{"prod-exact", "", false, 0},                       // current db.go config
		{"no-autock-no-bg", "&_pragma=wal_autocheckpoint(0)", false, 0},
		{"no-autock+passive", "&_pragma=wal_autocheckpoint(0)", true, 0},
		{"prod+long-reader", "", false, 3 * time.Second},   // long snapshot browser
		{"no-autock+long-reader", "&_pragma=wal_autocheckpoint(0)", false, 3 * time.Second},
	}

	for _, r := range runs {
		file := "prodshape_" + strings.ReplaceAll(r.name, "+", "_") + ".db"
		os.Remove(file)
		os.Remove(file + "-wal")
		os.Remove(file + "-shm")

		dsn := file +
			"?_pragma=journal_mode(WAL)" +
			"&_pragma=busy_timeout(5000)" +
			"&_pragma=synchronous(NORMAL)" +
			"&_pragma=cache_size(-20000)" +
			"&_pragma=journal_size_limit(4194304)" +
			"&_pragma=temp_store(MEMORY)" +
			r.extraPrag +
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

		db.Exec("CREATE TABLE IF NOT EXISTS prod (id INTEGER PRIMARY KEY, v TEXT)")
		tx := db.Begin()
		for i := 0; i < 12000; i++ {
			tx.Exec("INSERT INTO prod (v) VALUES (?)", strings.Repeat("x", 100))
		}
		tx.Commit()

		ctx := context.Background()
		stop := make(chan struct{})

		// background reader loop = API browsing (short snapshots)
		var readerWG sync.WaitGroup
		readerWG.Add(1)
		go func() {
			defer readerWG.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				conn, _ := sqlDB.Conn(ctx)
				if conn != nil {
					conn.ExecContext(ctx, "BEGIN")
					conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM prod").Scan(new(int))
					if r.readerKeep > 0 {
						time.Sleep(r.readerKeep)
					}
					conn.ExecContext(ctx, "ROLLBACK")
					conn.Close()
				}
				time.Sleep(2 * time.Millisecond)
			}
		}()

		// optional background PASSIVE checkpoint
		var ckWG sync.WaitGroup
		if r.ckp {
			ckWG.Add(1)
			go func() {
				defer ckWG.Done()
				for {
					select {
					case <-stop:
						return
					default:
					}
					sqlDB.Exec("PRAGMA wal_checkpoint(PASSIVE)")
					time.Sleep(100 * time.Millisecond)
				}
			}()
		}

		// 3 writers, 25 commits each; measure max commit time
		var mu sync.Mutex
		maxDur := time.Duration(0)
		fails := 0
		slow := 0
		var wg sync.WaitGroup
		for w := 0; w < 3; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				for i := 0; i < 25; i++ {
					wt := db.Begin()
					if wt.Error != nil {
						mu.Lock()
						fails++
						mu.Unlock()
						continue
					}
					wt.Exec("INSERT INTO prod (v) VALUES (?)", fmt.Sprint(w, i))
					t0 := time.Now()
					err := wt.Commit().Error
					d := time.Since(t0)
					mu.Lock()
					if d > maxDur {
						maxDur = d
					}
					if d > 100*time.Millisecond {
						slow++
					}
					if err != nil {
						fails++
					}
					mu.Unlock()
				}
			}(w)
		}
		wg.Wait()
		close(stop)
		readerWG.Wait()
		if r.ckp {
			ckWG.Wait()
		}
		sqlDB.Close()

		t.Logf("%-22s maxCommit=%s slowCommits>100ms=%d fails=%d", r.name, maxDur.Round(time.Millisecond), slow, fails)
	}
}