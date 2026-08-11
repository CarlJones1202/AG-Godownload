// Command importer migrates data from the legacy SQLite database (gallery.db,
// produced by the earlier sqlite-backed build) into PostgreSQL.
//
// Usage:
//
//	go run ./cmd/importer [-src gallery.db] [-dsn "host=... port=... user=... password=... dbname=gallery ..."]
//
// Both flags default to the application config: -src defaults to DATABASE_PATH
// (gallery.db) and -dsn defaults to DATABASE_URL / the composed PG* vars.
// The target PostgreSQL schema is created via GORM AutoMigrate, then every
// table is copied preserving primary keys and foreign key ids. Run against an
// empty (or freshly created) gallery database.
package main

import (
	"flag"
	"fmt"
	"gallery_api/config"
	"gallery_api/database"
	"gallery_api/logger"
	"gallery_api/models"
	"log/slog"
	"os"
	"reflect"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type tableDef struct {
	table string
	rows  any
}

func main() {
	srcPath := flag.String("src", "", "path to the legacy SQLite database (default: DATABASE_PATH)")
	dsn := flag.String("dsn", "", "PostgreSQL DSN (default: DATABASE_URL or PG* env)")
	flag.Parse()

	config.Load()
	if *srcPath == "" {
		*srcPath = config.Global.DatabasePath
	}
	if *dsn == "" {
		*dsn = config.Global.DatabaseDSN()
	}

	if _, err := os.Stat(*srcPath); err != nil {
		logger.Fatal("source sqlite database not found:", err)
	}

	src, err := gorm.Open(sqlite.Open(*srcPath), &gorm.Config{})
	if err != nil {
		logger.Fatal("failed to open sqlite database:", err)
	}

	// The source sqlite database was created without enforced foreign keys
	// (GORM/SQLite don't validate them), and rows use sentinel ids like
	// gallery_id = 0 for "no gallery". Reproduce that behavior on the target:
	// do not emit FK constraints, so orphan/sentinel references copy verbatim
	// exactly as they existed before.
	dst, err := gorm.Open(postgres.Open(*dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger: database.NewLogger(),
	})
	if err != nil {
		logger.Fatal("failed to open postgres database:", err)
	}

	if err := dst.AutoMigrate(
		&models.Source{},
		&models.Gallery{},
		&models.Image{},
		&models.Person{},
		&models.PersonIdentifier{},
		&models.PersonExclusion{},
		&models.PersonProviderAlias{},
		&models.PersonScanQueue{},
		&models.ScanResultExclusion{},
		&models.Tag{},
		&models.ImageEmbedding{},
		&models.ImageRating{},
		&models.EmbedQueue{},
	); err != nil {
		logger.Fatal("failed to migrate target schema:", err)
	}
	logger.Info("Target schema migrated (contains no data)")

	// Import the typed tables that own a numeric auto-increment id.
	importers := []tableDef{
		{"sources", &[]models.Source{}},
		{"galleries", &[]models.Gallery{}},
		{"people", &[]models.Person{}},
		{"tags", &[]models.Tag{}},
		{"images", &[]models.Image{}},
		{"person_identifiers", &[]models.PersonIdentifier{}},
		{"person_exclusions", &[]models.PersonExclusion{}},
		{"person_provider_aliases", &[]models.PersonProviderAlias{}},
		{"person_scan_queues", &[]models.PersonScanQueue{}},
		{"scan_result_exclusions", &[]models.ScanResultExclusion{}},
		{"image_embeddings", &[]models.ImageEmbedding{}},
		{"image_ratings", &[]models.ImageRating{}},
		{"embed_queues", &[]models.EmbedQueue{}},
	}

	for _, td := range importers {
		n, err := copyTable(src, dst, td.table, td.rows, 500)
		if err != nil {
			logger.Fatalf("import of %s failed: %v", td.table, err)
		}
		logger.Infof("Imported %d rows into %s", n, td.table)
		fixSequence(dst, td.table)
	}

	// Import GORM many2many junction tables (composite keys, no auto id).
	for _, table := range []string{"image_galleries", "person_galleries", "person_images", "image_tags"} {
		n, err := copyJunction(src, dst, table, 1000)
		if err != nil {
			logger.Fatalf("import of %s failed: %v", table, err)
		}
		logger.Infof("Imported %d rows into %s", n, table)
	}

	logger.Info("Import complete. Start the API with `go run .` after PostgreSQL is running.")
}

const batchSize = 500

// copyTable streams rows from a sqlite table into an equally-named postgres
// table while preserving ids. rows must be a pointer to a slice of a model.
func copyTable(src, dst *gorm.DB, table string, rows any, batch int) (int, error) {
	target := dst.Session(&gorm.Session{SkipHooks: true}).Table(table)
	lastID := uint64(0)
	total := 0
	for {
		r := reflect.New(reflect.TypeOf(rows).Elem()) // *[]T
		// Unscoped: soft-deleted rows must be migrated too so counts, ids and
		// any references to them are preserved exactly.
		if err := src.Unscoped().Table(table).Where("id > ?", lastID).Order("id ASC").Limit(batch).Find(r.Interface()).Error; err != nil {
			return total, fmt.Errorf("read: %w", err)
		}
		batchRows := r.Elem()
		if batchRows.Len() == 0 {
			return total, nil
		}
		if err := target.CreateInBatches(batchRows.Interface(), batch).Error; err != nil {
			return total, fmt.Errorf("write: %w", err)
		}
		last := batchRows.Index(batchRows.Len() - 1)
		lastID = last.FieldByName("ID").Uint()
		total += batchRows.Len()
	}
}

// copyJunction duplicates a composite-key junction table (ints only).
func copyJunction(src, dst *gorm.DB, table string, batch int) (int, error) {
	var rows []map[string]interface{}
	if err := src.Table(table).Find(&rows).Error; err != nil {
		return 0, fmt.Errorf("read: %w", err)
	}
	total := 0
	for i := 0; i < len(rows); i += batch {
		end := i + batch
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[i:end]
		if err := dst.Table(table).CreateInBatches(chunk, batch).Error; err != nil {
			return total, fmt.Errorf("write: %w", err)
		}
		total += len(chunk)
	}
	return total, nil
}

// fixSequence advances the auto-increment sequence past the max imported id so
// new inserts continue after the migrated rows.
func fixSequence(dst *gorm.DB, table string) {
	if err := dst.Exec(
		"SELECT setval(pg_get_serial_sequence(?, 'id'), GREATEST((SELECT COALESCE(MAX(id),1) FROM "+table+"), 1))",
		table).Error; err != nil {
		slog.Warn("sequence advance skipped", "table", table, "err", err)
	}
}
