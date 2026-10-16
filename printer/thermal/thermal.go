package thermal

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"

	"soreaja-printer/printer"

	soreaja "github.com/punkestu/soreaja-thermal"
	"github.com/punkestu/soreaja-thermal/pkg/escpos"
)

const (
	// ThermalWidth58mm is the standard printable pixel width for 58mm thermal printers (384 dots).
	ThermalWidth58mm = 384
	// ThermalWidth80mm is the standard printable pixel width for 80mm thermal printers (576 dots).
	ThermalWidth80mm = 576
	// DefaultThermalWidth is set to 80mm format (576 dots).
	DefaultThermalWidth = ThermalWidth80mm
)

// ThermalPrinter encodes and sends print jobs using soreaja-thermal Bluetooth transport or direct ESC/POS raster writing.
type ThermalPrinter struct {
	PrintableWidth int
	DefaultMAC     string
	mu             sync.Mutex
	printerClient  *soreaja.Client
}

// NewThermalPrinter creates a new instance of ThermalPrinter with optional default MAC address.
func NewThermalPrinter(defaultMAC ...string) *ThermalPrinter {
	mac := ""
	if len(defaultMAC) > 0 {
		mac = defaultMAC[0]
	}
	tp := &ThermalPrinter{
		PrintableWidth: DefaultThermalWidth,
		DefaultMAC:     mac,
	}
	if mac != "" {
		go func() {
			client, err := soreaja.Connect(mac, 1)
			if err == nil {
				tp.mu.Lock()
				tp.printerClient = client
				tp.mu.Unlock()
			}
		}()
	}
	return tp
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
	// Pre-scale image to 80mm format before sending to printer
	printFilePath, cleanup := t.prepareScaledImageFile(filePath)
	defer cleanup()

	type printResult struct {
		jobID string
		err   error
	}

	resCh := make(chan printResult, 1)

	go func() {
		client := t.printerClient
		if client == nil || !client.IsConnected() {
			var err error
			client, err = soreaja.Connect(macAddress, 1)
			if err != nil {
				resCh <- printResult{err: fmt.Errorf("failed to connect to thermal printer at %s: %w", macAddress, err)}
				return
			}
			t.printerClient = client
		}

		_ = client.Init()

		// Attempt to print as image first
		imgErr := client.PrintImage(printFilePath, t.PrintableWidth, true)
		if imgErr != nil {
			// If image rasterizing fails, attempt printing as raw text
			raw, readErr := os.ReadFile(printFilePath)
			if readErr != nil {
				resCh <- printResult{err: fmt.Errorf("failed to read print file: %v (image decode: %v)", readErr, imgErr)}
				return
			}
			if err := client.PrintText(string(raw)); err != nil {
				resCh <- printResult{err: fmt.Errorf("failed to print text to printer: %w", err)}
				return
			}
		}

		_ = client.Feed(1)
		_ = client.CutPaper(1)

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
	// Pre-scale image to 80mm format before rasterizing
	printFilePath, cleanup := t.prepareScaledImageFile(filePath)
	defer cleanup()

	chunks, err := escpos.RasterizeFile(printFilePath, t.PrintableWidth, true)
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
		raw, readErr := os.ReadFile(printFilePath)
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

// ScaleImageToWidth resizes an image to the target width while preserving the original aspect ratio.
func ScaleImageToWidth(src image.Image, targetWidth int) image.Image {
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return src
	}

	targetH := int(float64(srcH) * float64(targetWidth) / float64(srcW))
	if targetH <= 0 {
		targetH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetH))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	return dst
}

// prepareScaledImageFile scales the image at filePath to the configured thermal width (80mm / 576 dots)
// and returns the path to the scaled image file along with a cleanup closure.
func (t *ThermalPrinter) prepareScaledImageFile(filePath string) (string, func()) {
	noOpCleanup := func() {}

	file, err := os.Open(filePath)
	if err != nil {
		return filePath, noOpCleanup
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		// Not an image file (e.g. plain text or raw bytes), keep original
		return filePath, noOpCleanup
	}

	targetW := t.PrintableWidth
	if targetW <= 0 {
		targetW = ThermalWidth80mm
	}

	scaledImg := ScaleImageToWidth(img, targetW)

	tmpFile, err := os.CreateTemp("", "thermal-80mm-scaled-*.png")
	if err != nil {
		return filePath, noOpCleanup
	}

	if err := png.Encode(tmpFile, scaledImg); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return filePath, noOpCleanup
	}
	tmpFile.Close()

	cleanup := func() {
		_ = os.Remove(tmpFile.Name())
	}
	return tmpFile.Name(), cleanup
}

// ImageToESCPOS converts an image.Image into ESC/POS raster bit image commands using soreaja-thermal escpos engine.
func (t *ThermalPrinter) ImageToESCPOS(src image.Image) ([]byte, error) {
	targetW := t.PrintableWidth
	if targetW <= 0 || targetW%8 != 0 {
		targetW = ThermalWidth80mm
	}

	scaledSrc := ScaleImageToWidth(src, targetW)
	chunks := escpos.RasterizeImage(scaledSrc, targetW, true, 128)
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
