package color

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"soreaja-printer/printer"
)

// ColorPrinter executes print jobs via the local lp binary for color printers (such as Epson).
type ColorPrinter struct{}

// NewColorPrinter creates a new instance of ColorPrinter.
func NewColorPrinter() *ColorPrinter {
	return &ColorPrinter{}
}

// Print submits the file at filePath to the specified color printer using lp.
func (p *ColorPrinter) Print(ctx context.Context, printerName string, filePath string) (string, error) {
	if err := printer.ValidatePrinterName(printerName); err != nil {
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
		"-o", "page-left=0",
		"-o", "page-right=0",
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
	matches := printer.JobIDRegex.FindStringSubmatch(outStr)
	if len(matches) > 1 {
		return matches[1], nil
	}

	trimmed := strings.TrimSpace(outStr)
	if trimmed != "" {
		return trimmed, nil
	}

	return fmt.Sprintf("%s-queued", printerName), nil
}
