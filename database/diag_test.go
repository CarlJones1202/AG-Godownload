package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func qRow(t *testing.T, conn *sql.Conn, q string) string {
	t.Helper()
	var v string
	if err := conn.QueryRowContext(context.Background(), q).Scan(&v); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return v
}

// TestFastDiagnostic pins physical connections to prove the fundamentals:
// 1) is the database ACTUALLY in WAL mode? 2) do DSN pragmas apply?
// 3) does a long-lived reader block a single writer commit?
func TestFastDiagnostic(t *testing.T) {
	if os.Getenv("TEST_SQLITE_PROBES") != "1" {
		t.Skip("sqlite-specific probe; set TEST_SQLITE_PROBES=1 to run")
	}
	file := "diag.db"
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
	defer sqlDB.Close()

	ctx := context.Background()

	// 1) fundamentals
	connA, _ := sqlDB.Conn(ctx)
	defer connA.Close()
	t.Logf("journal_mode      = %q (want wal)", qRow(t, connA, "PRAGMA journal_mode"))
	t.Logf("busy_timeout      = %q", qRow(t, connA, "PRAGMA busy_timeout"))
	t.Logf("wal_autocheckpoint= %q", qRow(t, connA, "PRAGMA wal_autocheckpoint"))
	t.Logf("journal_size_limit= %q", qRow(t, connA, "PRAGMA journal_size_limit"))
	t.Logf("wal_autocheckpoint(0) -> %q", qRow(t, connA, "PRAGMA wal_autocheckpoint=0"))
	t.Logf("wal_autocheckpoint now= %q", qRow(t, connA, "PRAGMA wal_autocheckpoint"))
	t.Logf("wal_autocheckpoint back= %q", qRow(t, connA, "PRAGMA wal_autocheckpoint=1000"))

	// 2) seed ~15k rows (single tx, ends before reader starts)
	db.Exec("DROP TABLE IF EXISTS diag")
	db.Exec("CREATE TABLE diag (id INTEGER PRIMARY KEY, v TEXT)")
	tx := db.Begin()
	for i := 0; i < 15000; i++ {
		if err := tx.Exec("INSERT INTO diag (v) VALUES (?)", strings.Repeat("x", 100)).Error; err != nil {
			tx.Rollback()
			t.Fatalf("seed: %v", err)
		}
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("seed commit: %v", err)
	}
	wal, _ := os.Stat(file + "-wal")
	t.Logf("WAL file size after 15k-row seed = %d bytes", wal.Size())

	// 3) reader holds snapshot on connB; writer on connC
	connB, _ := sqlDB.Conn(ctx)
	defer connB.Close()
	connC, _ := sqlDB.Conn(ctx)
	defer connC.Close()

	if _, err := connB.ExecContext(ctx, "BEGIN"); err != nil {
		t.Fatalf("reader begin: %v", err)
	}
	var cnt int
	if err := connB.QueryRowContext(ctx, "SELECT COUNT(*) FROM diag").Scan(&cnt); err != nil {
		t.Fatalf("reader query: %v", err)
	}
	t.Logf("reader snapshot active (rows=%d)", cnt)

	t0 := time.Now()
	if _, err := connC.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("writer BEGIN IMMEDIATE while reader active: %v", err)
	}
	if _, err := connC.ExecContext(ctx, "INSERT INTO diag (v) VALUES ('probe')"); err != nil {
		t.Fatalf("writer insert while reader active: %v", err)
	}
	if _, err := connC.ExecContext(ctx, "COMMIT"); err != nil {
		t.Fatalf("writer commit while reader active: %v", err)
	}
	t.Logf("writer (BEGIN IMMEDIATE+INSERT+COMMIT) with reader active took %s — NO BLOCK", time.Since(t0).Round(time.Millisecond))

	if _, err := connB.ExecContext(ctx, "ROLLBACK"); err != nil {
		t.Fatalf("reader rollback: %v", err)
	}
	t.Logf("reader rolled back, all good: %s", fmt.Sprint(cnt))
}
