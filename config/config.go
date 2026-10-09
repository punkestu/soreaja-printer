package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds the application configuration loaded from environment variables.
type Config struct {
	ServerPort         string
	DefaultPrinterName string
	MaxUploadSize      int64 // in bytes
	PrintTimeout       time.Duration
}

// Load reads configuration from environment variables, falling back to sensible defaults.
func Load() *Config {
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}

	defaultPrinter := os.Getenv("DEFAULT_PRINTER_NAME")
	if defaultPrinter == "" {
		defaultPrinter = "EPSON_L120_Series"
	}

	maxUploadBytes := int64(10 << 20) // 10MB default
	if maxUploadStr := os.Getenv("MAX_UPLOAD_SIZE_BYTES"); maxUploadStr != "" {
		if val, err := strconv.ParseInt(maxUploadStr, 10, 64); err == nil && val > 0 {
			maxUploadBytes = val
		}
	}

	timeout := 30 * time.Second
	if timeoutStr := os.Getenv("PRINT_TIMEOUT_SECONDS"); timeoutStr != "" {
		if sec, err := strconv.Atoi(timeoutStr); err == nil && sec > 0 {
			timeout = time.Duration(sec) * time.Second
		}
	}

	return &Config{
		ServerPort:         port,
		DefaultPrinterName: defaultPrinter,
		MaxUploadSize:      maxUploadBytes,
		PrintTimeout:       timeout,
	}
}
