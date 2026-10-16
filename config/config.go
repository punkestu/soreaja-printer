package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds the application configuration loaded from environment variables.
type Config struct {
	ServerPort                string
	DefaultColorPrinterName   string
	DefaultThermalPrinterName string
	DefaultThermalPrinterMAC  string
	MaxUploadSize             int64 // in bytes
	PrintTimeout              time.Duration
}

// Load reads configuration from environment variables, falling back to sensible defaults.
func Load() *Config {
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}

	defaultColorPrinter := os.Getenv("DEFAULT_COLOR_PRINTER_NAME")
	if defaultColorPrinter == "" {
		defaultColorPrinter = "EPSON_L120_Series"
	}

	defaultThermalPrinter := os.Getenv("DEFAULT_THERMAL_PRINTER_NAME")
	if defaultThermalPrinter == "" {
		defaultThermalPrinter = "/dev/cu.RPP02N"
	}

	defaultThermalPrinterMAC := os.Getenv("DEFAULT_THERMAL_PRINTER_MAC")
	if defaultThermalPrinterMAC == "" {
		defaultThermalPrinterMAC = "86:67:7A:7C:0A:56"
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
		ServerPort:                port,
		DefaultColorPrinterName:   defaultColorPrinter,
		DefaultThermalPrinterName: defaultThermalPrinter,
		DefaultThermalPrinterMAC:  defaultThermalPrinterMAC,
		MaxUploadSize:             maxUploadBytes,
		PrintTimeout:              timeout,
	}
}
