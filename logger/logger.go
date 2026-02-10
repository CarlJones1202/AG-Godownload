package logger

import (
	"log"
	"os"
	"strings"
)

type LogLevel int

const (
	TRACE LogLevel = iota
	DEBUG
	INFO
	WARN
	ERROR
)

var (
	currentLevel LogLevel
	logPrefix    = map[LogLevel]string{
		TRACE: "[TRACE] ",
		DEBUG: "[DEBUG] ",
		INFO:  "[INFO]  ",
		WARN:  "[WARN]  ",
		ERROR: "[ERROR] ",
	}
)

func init() {
	// Default to INFO level (standard default)
	currentLevel = INFO

	// Allow override via environment variable
	if level := os.Getenv("LOG_LEVEL"); level != "" {
		SetLevelFromString(level)
	}
}

func SetLevel(level LogLevel) {
	currentLevel = level
}

func SetLevelFromString(level string) {
	switch strings.ToUpper(level) {
	case "TRACE":
		currentLevel = TRACE
	case "DEBUG":
		currentLevel = DEBUG
	case "INFO":
		currentLevel = INFO
	case "WARN", "WARNING":
		currentLevel = WARN
	case "ERROR":
		currentLevel = ERROR
	default:
		log.Printf("Unknown log level: %s, defaulting to WARN\n", level)
		currentLevel = WARN
	}
}

func shouldLog(level LogLevel) bool {
	return level >= currentLevel
}

func Trace(v ...interface{}) {
	if shouldLog(TRACE) {
		log.Print(logPrefix[TRACE], v)
	}
}

func Tracef(format string, v ...interface{}) {
	if shouldLog(TRACE) {
		log.Printf(logPrefix[TRACE]+format, v...)
	}
}

func Debug(v ...interface{}) {
	if shouldLog(DEBUG) {
		log.Print(logPrefix[DEBUG], v)
	}
}

func Debugf(format string, v ...interface{}) {
	if shouldLog(DEBUG) {
		log.Printf(logPrefix[DEBUG]+format, v...)
	}
}

func Info(v ...interface{}) {
	if shouldLog(INFO) {
		log.Print(logPrefix[INFO], v)
	}
}

func Infof(format string, v ...interface{}) {
	if shouldLog(INFO) {
		log.Printf(logPrefix[INFO]+format, v...)
	}
}

func Warn(v ...interface{}) {
	if shouldLog(WARN) {
		log.Print(logPrefix[WARN], v)
	}
}

func Warnf(format string, v ...interface{}) {
	if shouldLog(WARN) {
		log.Printf(logPrefix[WARN]+format, v...)
	}
}

func Error(v ...interface{}) {
	if shouldLog(ERROR) {
		log.Print(logPrefix[ERROR], v)
	}
}

func Errorf(format string, v ...interface{}) {
	if shouldLog(ERROR) {
		log.Printf(logPrefix[ERROR]+format, v...)
	}
}

// Fatal always logs and exits
func Fatal(v ...interface{}) {
	log.Fatal(logPrefix[ERROR], v)
}

func Fatalf(format string, v ...interface{}) {
	log.Fatalf(logPrefix[ERROR]+format, v...)
}
