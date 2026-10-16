package printer

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

var (
	// printerNameRegex ensures printer name only contains alphanumeric characters, underscores, and hyphens.
	printerNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	// safePathRegex permits safe absolute file or device paths (e.g. /dev/cu.RPP02N, /dev/ttyUSB0, or output files).
	safePathRegex = regexp.MustCompile(`^/[a-zA-Z0-9_./-]+$`)
	// netAddrRegex permits network printer addresses such as 192.168.1.100:9100.
	netAddrRegex = regexp.MustCompile(`^[a-zA-Z0-9_.-]+:[0-9]{1,5}$`)
	// macAddrRegex permits standard Bluetooth or network MAC addresses (e.g. AA:BB:CC:DD:EE:FF or AA-BB-CC-DD-EE-FF).
	macAddrRegex = regexp.MustCompile(`^([0-9a-fA-F]{2}[:-]){5}([0-9a-fA-F]{2})$`)
	// JobIDRegex extracts the print job ID from standard lp command output.
	JobIDRegex = regexp.MustCompile(`request id is\s+([^\s]+)`)
)

// ErrInvalidPrinterName is returned when a printer name contains forbidden characters.
var ErrInvalidPrinterName = errors.New("invalid printer name: only alphanumeric, hyphen, underscore, valid device path/address, or MAC address allowed")

// IsMACAddress returns true if the string is formatted as a valid MAC address.
func IsMACAddress(name string) bool {
	return macAddrRegex.MatchString(strings.TrimSpace(name))
}

// ValidatePrinterName checks if the printer name adheres to safe naming standards.
func ValidatePrinterName(name string) error {
	if name == "" || strings.Contains(name, "..") {
		return ErrInvalidPrinterName
	}
	if printerNameRegex.MatchString(name) || safePathRegex.MatchString(name) || netAddrRegex.MatchString(name) || macAddrRegex.MatchString(name) {
		return nil
	}
	return ErrInvalidPrinterName
}

// Service defines the contract for sending print jobs.
type Service interface {
	Print(ctx context.Context, printerName string, filePath string) (string, error)
}
