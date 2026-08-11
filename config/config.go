package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Server
	Port string
	env  string

	// Database
	DatabasePath string
	LogLevel     string

	// Database (PostgreSQL)
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	PGBinDir   string // directory containing pg_dump / pg_ctl etc. (set by init.ps1)

	// Workers
	CrawlerWorkers int
	AITagWorkers   int

	// Images
	UploadsDir             string
	MaxConcurrentDownloads int

	// GalleryDL fallback
	GalleryDL struct {
		Enabled    bool
		BinaryPath string
		TimeoutSec int
		Providers  []string
	}

	// HTTP Client
	HTTPConnectTimeout time.Duration
	HTTPRequestTimeout time.Duration
	HTTPMaxRetries     int
	// Maintenance token for privileged API endpoints
	MaintenanceToken string

	// Embedding / content similarity
	Embedding struct {
		ModelPath    string // ONNX model path; empty = built-in low-level provider
		ModelName    string // embedder version tag stored in image_embeddings
		Dim          int    // expected vector dimension
		TagThreshold float64
		Concurrency  int
		// Recommendation blending weights: embed,tags,color,pref (must sum to 1)
		RecWeights        [4]float64
		RecDiversityLambda float64
	}
}

var Global Config

func Load() {
	_ = godotenv.Load()
	Global = Config{
		Port:           getEnv("PORT", "8080"),
		env:            getEnv("ENV", "development"),
		DatabasePath:   getEnv("DATABASE_PATH", "gallery.db"),
		LogLevel:       getEnv("LOG_LEVEL", "INFO"),
		DBHost:         getEnv("PGHOST", "127.0.0.1"),
		DBPort:         getEnv("PGPORT", "5432"),
		DBUser:         getEnv("PGUSER", "postgres"),
		DBPassword:     getEnv("PGPASSWORD", "postgres"),
		DBName:         getEnv("PGDATABASE", "gallery"),
		PGBinDir:       getEnv("PGBIN", ""),
		CrawlerWorkers: getEnvAsInt("CRAWLER_WORKERS", 5),
		// Default to 0 to disable AI tagging unless explicitly enabled
		AITagWorkers:           getEnvAsInt("AITAG_WORKERS", 0),
		UploadsDir:             getEnv("UPLOADS_DIR", "uploads"),
		MaxConcurrentDownloads: getEnvAsInt("MAX_CONCURRENT_DOWNLOADS", 10),
		HTTPConnectTimeout:     time.Duration(getEnvAsInt("HTTP_CONNECT_TIMEOUT_SEC", 10)) * time.Second,
		HTTPRequestTimeout:     time.Duration(getEnvAsInt("HTTP_REQUEST_TIMEOUT_SEC", 30)) * time.Second,
		HTTPMaxRetries:         getEnvAsInt("HTTP_MAX_RETRIES", 3),
		MaintenanceToken:       getEnv("MAINTENANCE_TOKEN", ""),
	}

	// GalleryDL defaults
	Global.GalleryDL.Enabled = false
	Global.GalleryDL.BinaryPath = getEnv("GALLERYDL_BINARY", "gallery-dl")
	Global.GalleryDL.TimeoutSec = getEnvAsInt("GALLERYDL_TIMEOUT_SEC", 30)
	// Providers default - only imx
	Global.GalleryDL.Providers = []string{"imx"}

	// Embedding defaults — feature is always on via the built-in low-level
	// provider; setting EMBED_MODEL_PATH opts into a semantic embedder later.
	Global.Embedding.ModelPath = getEnv("EMBED_MODEL_PATH", "")
	Global.Embedding.ModelName = getEnv("EMBED_MODEL_NAME", "lowlevel-v1")
	Global.Embedding.Dim = getEnvAsInt("EMBED_DIM", 51)
	Global.Embedding.TagThreshold = 0.3
	if v, err := strconv.ParseFloat(getEnv("EMBED_TAG_THRESHOLD", "0.3"), 64); err == nil {
		Global.Embedding.TagThreshold = v
	}
	Global.Embedding.Concurrency = getEnvAsInt("EMBED_CONCURRENCY", 2)
	if Global.Embedding.Concurrency < 1 {
		Global.Embedding.Concurrency = 1
	}

	// Recommendation weights: embed, tags, color, pref
	Global.Embedding.RecWeights = [4]float64{0.5, 0.25, 0.15, 0.10}
	if w := getEnv("REC_WEIGHTS", ""); w != "" {
		if parts := strings.Split(w, ","); len(parts) == 4 {
			parsed := [4]float64{}
			ok := true
			for i, p := range parts {
				v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
				if err != nil {
					ok = false
					break
				}
				parsed[i] = v
			}
			if ok {
				sum := parsed[0] + parsed[1] + parsed[2] + parsed[3]
				if sum > 0 {
					for i := range parsed {
						parsed[i] /= sum
					}
					Global.Embedding.RecWeights = parsed
				}
			}
		}
	}
	Global.Embedding.RecDiversityLambda = getEnvAsFloat("REC_DIVERSITY_LAMBDA", 0.6)
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	strValue := getEnv(key, "")
	if strValue == "" {
		return fallback
	}
	if value, err := strconv.Atoi(strValue); err == nil {
		return value
	}
	return fallback
}

func getEnvAsFloat(key string, fallback float64) float64 {
	strValue := getEnv(key, "")
	if strValue == "" {
		return fallback
	}
	if value, err := strconv.ParseFloat(strValue, 64); err == nil {
		return value
	}
	return fallback
}

func IsDev() bool {
	return Global.env == "development"
}

// DatabaseDSN returns a PostgreSQL connection string. When DATABASE_URL is set
// it is used verbatim (also honored by the sqlite->pg importer and pg_dump);
// otherwise the discrete PG* options are composed into a DSN.
func (c *Config) DatabaseDSN() string {
	if d := getEnv("DATABASE_URL", ""); d != "" {
		return d
	}
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName)
}
