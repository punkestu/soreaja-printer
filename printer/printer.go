package printer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var (
	// printerNameRegex ensures printer name only contains alphanumeric characters, underscores, and hyphens.
	printerNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	// jobIdRegex extracts the print job ID from standard lp command output (e.g. "request id is EPSON_L120_Series-123 (1 file(s))").
	jobIDRegex = regexp.MustCompile(`request id is\s+([^\s]+)`)
)

// ErrInvalidPrinterName is returned when a printer name contains forbidden characters.
var ErrInvalidPrinterName = errors.New("invalid printer name: only alphanumeric, hyphen, and underscore characters are allowed")

// ValidatePrinterName checks if the printer name adheres to safe naming standards.
func ValidatePrinterName(name string) error {
	if name == "" || !printerNameRegex.MatchString(name) {
		return ErrInvalidPrinterName
	}
	return nil
}

// Service defines the contract for sending print jobs.
type Service interface {
	Print(ctx context.Context, printerName string, filePath string) (string, error)
}

// CUPSPrinter executes print jobs via the local lp binary.
type CUPSPrinter struct{}

// NewCUPSPrinter creates a new instance of CUPSPrinter.
func NewCUPSPrinter() *CUPSPrinter {
	return &CUPSPrinter{}
}

// Print submits the file at filePath to the specified printer using lp.
func (p *CUPSPrinter) Print(ctx context.Context, printerName string, filePath string) (string, error) {
	if err := ValidatePrinterName(printerName); err != nil {
		return "", err
	}

	args := []string{
		"-d", printerName,
		"-o", "PageSize=Custom.288x432",
		"-o", "Resolution=720x720dpi",
		"-o", "EPIJ_Qual=306",
		"-o", "EPIJ_CMat=3",
		"-o", "EPIJ_AFil=1",
		"-o", "fit-to-page",
		filePath,
	}

	cmd := exec.CommandContext(ctx, "lp", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errOutput := strings.TrimSpace(stderr.String())
		if errOutput == "" {
			errOutput = strings.TrimSpace(stdout.String())
		}
		if errOutput != "" {
			return "", fmt.Errorf("lp failed (%w): %s", err, errOutput)
		}
		return "", fmt.Errorf("lp failed: %w", err)
	}

	outStr := stdout.String()
	matches := jobIDRegex.FindStringSubmatch(outStr)
	if len(matches) > 1 {
		return matches[1], nil
	}

	trimmed := strings.TrimSpace(outStr)
	if trimmed != "" {
		return trimmed, nil
	}

	return fmt.Sprintf("%s-queued", printerName), nil
}
