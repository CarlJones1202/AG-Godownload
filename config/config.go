package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	// Server
	Port string
	env  string

	// Database
	DatabasePath string
	LogLevel     string

	// Workers
	CrawlerWorkers int
	AITagWorkers   int

	// Images
	UploadsDir             string
	MaxConcurrentDownloads int

	// HTTP Client
	HTTPConnectTimeout time.Duration
	HTTPRequestTimeout time.Duration
	HTTPMaxRetries     int
	// Maintenance token for privileged API endpoints
	MaintenanceToken string
}

var Global Config

func Load() {
	Global = Config{
		Port:                   getEnv("PORT", "8080"),
		env:                    getEnv("ENV", "development"),
		DatabasePath:           getEnv("DATABASE_PATH", "gallery.db"),
		LogLevel:               getEnv("LOG_LEVEL", "INFO"),
		CrawlerWorkers:         getEnvAsInt("CRAWLER_WORKERS", 5),
		AITagWorkers:           getEnvAsInt("AITAG_WORKERS", 1),
		UploadsDir:             getEnv("UPLOADS_DIR", "uploads"),
		MaxConcurrentDownloads: getEnvAsInt("MAX_CONCURRENT_DOWNLOADS", 10),
		HTTPConnectTimeout:     time.Duration(getEnvAsInt("HTTP_CONNECT_TIMEOUT_SEC", 10)) * time.Second,
		HTTPRequestTimeout:     time.Duration(getEnvAsInt("HTTP_REQUEST_TIMEOUT_SEC", 30)) * time.Second,
		HTTPMaxRetries:         getEnvAsInt("HTTP_MAX_RETRIES", 3),
		MaintenanceToken:       getEnv("MAINTENANCE_TOKEN", ""),
	}
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

func IsDev() bool {
	return Global.env == "development"
}
