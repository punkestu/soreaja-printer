package thermal

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	soreaja "github.com/punkestu/soreaja-thermal"
	"github.com/punkestu/soreaja-thermal/pkg/escpos"
	"soreaja-printer/printer"
)

const (
	// DefaultThermalWidth is the standard printable pixel width for 58mm thermal printers (384 dots).
	DefaultThermalWidth = 384
)

// ThermalPrinter encodes and sends print jobs using soreaja-thermal Bluetooth transport or direct ESC/POS raster writing.
type ThermalPrinter struct {
	PrintableWidth int
	DefaultMAC     string
	mu             sync.Mutex
}

// NewThermalPrinter creates a new instance of ThermalPrinter with optional default MAC address.
func NewThermalPrinter(defaultMAC ...string) *ThermalPrinter {
	mac := ""
	if len(defaultMAC) > 0 {
		mac = defaultMAC[0]
	}
	return &ThermalPrinter{
		PrintableWidth: DefaultThermalWidth,
		DefaultMAC:     mac,
	}
}

// Print converts the file at filePath into ESC/POS commands and sends it to the target thermal printer.
func (t *ThermalPrinter) Print(ctx context.Context, printerTarget string, filePath string) (string, error) {
	printerTarget = strings.TrimSpace(printerTarget)
	if printerTarget == "" || strings.EqualFold(printerTarget, "thermal") || strings.EqualFold(printerTarget, "default") {
		if t.DefaultMAC != "" {
			printerTarget = t.DefaultMAC
		}
	}

	if err := printer.ValidatePrinterName(printerTarget); err != nil {
		return "", err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// If destination is a Bluetooth MAC address, use soreaja-thermal transport
	if printer.IsMACAddress(printerTarget) {
		return t.printViaSoreaja(ctx, printerTarget, filePath)
	}

	// Direct device path, test output file, or network address fallback
	return t.printViaDirectIO(ctx, printerTarget, filePath)
}

// printViaSoreaja connects to the printer via Bluetooth MAC address using github.com/punkestu/soreaja-thermal.
func (t *ThermalPrinter) printViaSoreaja(ctx context.Context, macAddress string, filePath string) (string, error) {
	type printResult struct {
		jobID string
		err   error
	}

	resCh := make(chan printResult, 1)

	go func() {
		client, err := soreaja.Connect(macAddress, 1)
		if err != nil {
			resCh <- printResult{err: fmt.Errorf("failed to connect to thermal printer at %s: %w", macAddress, err)}
			return
		}
		defer client.Disconnect()

		_ = client.Init()

		// Attempt to print as image first
		imgErr := client.PrintImage(filePath, t.PrintableWidth, true)
		if imgErr != nil {
			// If image rasterizing fails, attempt printing as raw text
			raw, readErr := os.ReadFile(filePath)
			if readErr != nil {
				resCh <- printResult{err: fmt.Errorf("failed to read print file: %v (image decode: %v)", readErr, imgErr)}
				return
			}
			if err := client.PrintText(string(raw)); err != nil {
				resCh <- printResult{err: fmt.Errorf("failed to print text to printer: %w", err)}
				return
			}
		}

		_ = client.Feed(3)
		_ = client.CutPaper(3)

		resCh <- printResult{jobID: fmt.Sprintf("thermal-mac-%d", time.Now().UnixNano())}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-resCh:
		return res.jobID, res.err
	}
}

// printViaDirectIO handles writing ESC/POS raster chunks directly to a device path, file, or network socket.
func (t *ThermalPrinter) printViaDirectIO(ctx context.Context, dest string, filePath string) (string, error) {
	chunks, err := escpos.RasterizeFile(filePath, t.PrintableWidth, true)
	var payload []byte
	if err == nil && len(chunks) > 0 {
		var buf bytes.Buffer
		buf.Write(escpos.Init())
		buf.Write(escpos.Align(escpos.AlignCenter))
		for _, chunk := range chunks {
			buf.Write(chunk)
		}
		buf.Write(escpos.Feed(4))
		payload = buf.Bytes()
	} else {
		// Plain text or raw bytes fallback
		raw, readErr := os.ReadFile(filePath)
		if readErr != nil {
			return "", readErr
		}
		var buf bytes.Buffer
		buf.Write(escpos.Init())
		buf.Write(raw)
		buf.Write([]byte{0x0D, 0x0A, 0x0D, 0x0A, 0x0D, 0x0A})
		payload = buf.Bytes()
	}

	if err := t.writePayload(ctx, dest, payload); err != nil {
		return "", fmt.Errorf("failed to write to thermal printer '%s': %w", dest, err)
	}

	return fmt.Sprintf("thermal-%d", time.Now().UnixNano()), nil
}

// ImageToESCPOS converts an image.Image into ESC/POS raster bit image commands using soreaja-thermal escpos engine.
func (t *ThermalPrinter) ImageToESCPOS(src image.Image) ([]byte, error) {
	targetW := t.PrintableWidth
	if targetW <= 0 || targetW%8 != 0 {
		targetW = DefaultThermalWidth
	}

	chunks := escpos.RasterizeImage(src, targetW, true, 128)
	if len(chunks) == 0 {
		return nil, fmt.Errorf("failed to encode ESC/POS image: empty raster output")
	}

	var buf bytes.Buffer
	buf.Write(escpos.Init())
	buf.Write(escpos.Align(escpos.AlignCenter))
	for _, chunk := range chunks {
		buf.Write(chunk)
	}
	buf.Write(escpos.Feed(4))

	return buf.Bytes(), nil
}

// writePayload sends the byte payload directly to a device path or network socket.
func (t *ThermalPrinter) writePayload(ctx context.Context, printerName string, data []byte) error {
	// 1. Network socket (e.g. 192.168.1.100:9100)
	if strings.Contains(printerName, ":") && !printer.IsMACAddress(printerName) {
		var d net.Dialer
		conn, err := d.DialContext(ctx, "tcp", printerName)
		if err != nil {
			return fmt.Errorf("failed to dial thermal printer at %s: %w", printerName, err)
		}
		defer conn.Close()

		_, err = conn.Write(data)
		return err
	}

	// 2. Direct device path (e.g. /dev/cu.RPP02N or file)
	var devFile *os.File
	var err error

	for attempt := 0; attempt < 5; attempt++ {
		devFile, err = os.OpenFile(printerName, os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			devFile, err = os.OpenFile(printerName, os.O_RDWR|os.O_CREATE, 0666)
		}
		if err == nil {
			break
		}
		if strings.Contains(strings.ToLower(err.Error()), "busy") {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		break
	}
	if err != nil {
		return fmt.Errorf("failed to open device %s: %w", printerName, err)
	}
	defer devFile.Close()

	done := make(chan error, 1)
	go func() {
		_, writeErr := devFile.Write(data)
		done <- writeErr
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case writeErr := <-done:
		return writeErr
	}
}
