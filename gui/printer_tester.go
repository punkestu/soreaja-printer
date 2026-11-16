package gui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"soreaja-printer/printer"
)

// PrinterTester provides diagnostic utilities to test color and thermal printer connections.
type PrinterTester struct {
	colorPrinter   printer.Service
	thermalPrinter printer.Service
}

// NewPrinterTester creates a new PrinterTester.
func NewPrinterTester(colorPrinter printer.Service, thermalPrinter printer.Service) *PrinterTester {
	return &PrinterTester{
		colorPrinter:   colorPrinter,
		thermalPrinter: thermalPrinter,
	}
}

// CheckColorStatus executes `lpstat -p` to verify CUPS queue status.
func (pt *PrinterTester) CheckColorStatus(ctx context.Context, printerName string) (string, error) {
	printerName = strings.TrimSpace(printerName)
	if printerName == "" {
		return "", fmt.Errorf("printer name cannot be empty")
	}
	if err := printer.ValidatePrinterName(printerName); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, "lpstat", "-p", printerName)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	output := strings.TrimSpace(outBuf.String())
	errOut := strings.TrimSpace(errBuf.String())

	if err != nil {
		if errOut != "" {
			return "", fmt.Errorf("lpstat error: %s", errOut)
		}
		if output != "" {
			return output, nil
		}
		return "", fmt.Errorf("lpstat failed (%w): printer queue may not exist in CUPS", err)
	}

	if output == "" {
		return fmt.Sprintf("Printer '%s' query completed with empty status", printerName), nil
	}

	return output, nil
}

// TestColorPrint generates a test swatch card and submits it to the color printer.
func (pt *PrinterTester) TestColorPrint(ctx context.Context, printerName string) (string, error) {
	printerName = strings.TrimSpace(printerName)
	if printerName == "" {
		return "", fmt.Errorf("printer name cannot be empty")
	}

	testImg := GenerateColorTestImage(printerName)
	tmpFile, err := os.CreateTemp("", "color-test-*.png")
	if err != nil {
		return "", fmt.Errorf("failed to create temp test file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if err := png.Encode(tmpFile, testImg); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to encode test image: %w", err)
	}
	tmpFile.Close()

	return pt.colorPrinter.Print(ctx, printerName, tmpFile.Name())
}

// CheckThermalStatus inspects the specified thermal printer target (Bluetooth MAC, socket, or device path).
func (pt *PrinterTester) CheckThermalStatus(ctx context.Context, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("thermal printer target cannot be empty")
	}

	if printer.IsMACAddress(target) {
		return fmt.Sprintf("Valid Bluetooth MAC address format: %s (Ready for connection)", target), nil
	}

	// Network socket (IP:Port)
	if strings.Contains(target, ":") {
		d := net.Dialer{Timeout: 3 * time.Second}
		conn, err := d.DialContext(ctx, "tcp", target)
		if err != nil {
			return "", fmt.Errorf("failed to reach thermal network printer at %s: %w", target, err)
		}
		_ = conn.Close()
		return fmt.Sprintf("Successfully connected to thermal network printer at %s", target), nil
	}

	// Device path (/dev/...)
	if strings.HasPrefix(target, "/dev/") {
		info, err := os.Stat(target)
		if err != nil {
			return "", fmt.Errorf("device path '%s' not accessible: %w", target, err)
		}
		return fmt.Sprintf("Device '%s' exists (mode: %s)", target, info.Mode().String()), nil
	}

	return fmt.Sprintf("Target '%s' is configured as printer identifier", target), nil
}

// TestThermalPrint generates a test receipt image and submits it to the thermal printer.
func (pt *PrinterTester) TestThermalPrint(ctx context.Context, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("thermal printer target cannot be empty")
	}

	testImg := GenerateThermalTestImage(target)
	tmpFile, err := os.CreateTemp("", "thermal-test-*.png")
	if err != nil {
		return "", fmt.Errorf("failed to create temp test file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if err := png.Encode(tmpFile, testImg); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to encode test image: %w", err)
	}
	tmpFile.Close()

	return pt.thermalPrinter.Print(ctx, target, tmpFile.Name())
}

// GenerateColorTestImage creates a colorful diagnostic bitmap image (500x350).
func GenerateColorTestImage(printerName string) image.Image {
	w, h := 500, 350
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	// Fill background with white
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{white}, image.Point{}, draw.Src)

	// Draw color test stripes (Cyan, Magenta, Yellow, Black, Red, Green, Blue)
	colors := []color.RGBA{
		{R: 0, G: 255, B: 255, A: 255},   // Cyan
		{R: 255, G: 0, B: 255, A: 255},   // Magenta
		{R: 255, G: 255, B: 0, A: 255},   // Yellow
		{R: 30, G: 30, B: 30, A: 255},    // Black/Dark Gray
		{R: 220, G: 50, B: 50, A: 255},   // Red
		{R: 40, G: 180, B: 40, A: 255},   // Green
		{R: 40, G: 80, B: 220, A: 255},   // Blue
	}

	stripeWidth := (w - 40) / len(colors)
	for i, c := range colors {
		x0 := 20 + i*stripeWidth
		x1 := x0 + stripeWidth - 4
		rect := image.Rect(x0, 30, x1, 100)
		draw.Draw(img, rect, &image.Uniform{c}, image.Point{}, draw.Src)
	}

	// Border around canvas
	black := color.RGBA{R: 0, G: 0, B: 0, A: 255}
	drawRectBorder(img, image.Rect(10, 10, w-10, h-10), black, 2)

	// Draw diagnostic text
	addLabel(img, 30, 140, "SOREAJA PRINTER - COLOR DIAGNOSTIC TEST", black)
	addLabel(img, 30, 175, fmt.Sprintf("Printer Queue: %s", printerName), black)
	addLabel(img, 30, 205, fmt.Sprintf("Timestamp: %s", time.Now().Format("2006-01-02 15:04:05 MST")), black)
	addLabel(img, 30, 235, "Status: Alignment & Color Rendition Verified OK", black)
	addLabel(img, 30, 275, "--------------------------------------------------------", black)
	addLabel(img, 30, 305, "Powered by soreaja-printer microservice", black)

	return img
}

// GenerateThermalTestImage creates a high-contrast black-and-white receipt test card (384x300).
func GenerateThermalTestImage(target string) image.Image {
	w, h := 384, 320
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	// White background
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{white}, image.Point{}, draw.Src)

	black := color.RGBA{R: 0, G: 0, B: 0, A: 255}

	// Top banner bar
	draw.Draw(img, image.Rect(10, 10, w-10, 45), &image.Uniform{black}, image.Point{}, draw.Src)
	addLabel(img, 25, 32, "=== SOREAJA THERMAL TEST ===", white)

	// Content text
	addLabel(img, 15, 70, "THERMAL PRINTER DIAGNOSTIC", black)
	addLabel(img, 15, 100, fmt.Sprintf("Target: %s", target), black)
	addLabel(img, 15, 130, fmt.Sprintf("Time:   %s", time.Now().Format("15:04:05 02-Jan-2006")), black)
	addLabel(img, 15, 160, "Raster: 384 / 576 dots compatible", black)
	addLabel(img, 15, 190, "Feed & Cutter Verification: OK", black)

	// Checkerboard pattern to verify dot density
	patternRect := image.Rect(15, 215, w-15, 255)
	drawRectBorder(img, patternRect, black, 1)
	for y := 218; y < 252; y += 4 {
		for x := 18; x < w-18; x += 4 {
			if ((x/4)+(y/4))%2 == 0 {
				img.Set(x, y, black)
				img.Set(x+1, y, black)
				img.Set(x, y+1, black)
				img.Set(x+1, y+1, black)
			}
		}
	}

	addLabel(img, 15, 280, "- - - - - - - - - - - - - - - - - - - -", black)
	addLabel(img, 50, 305, "THANK YOU FOR USING SOREAJA", black)

	return img
}

func addLabel(img *image.RGBA, x, y int, label string, col color.Color) {
	point := fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)}
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13,
		Dot:  point,
	}
	d.DrawString(label)
}

func drawRectBorder(img *image.RGBA, r image.Rectangle, col color.Color, thickness int) {
	for t := 0; t < thickness; t++ {
		// Top & bottom
		for x := r.Min.X; x < r.Max.X; x++ {
			img.Set(x, r.Min.Y+t, col)
			img.Set(x, r.Max.Y-1-t, col)
		}
		// Left & right
		for y := r.Min.Y; y < r.Max.Y; y++ {
			img.Set(r.Min.X+t, y, col)
			img.Set(r.Max.X-1-t, y, col)
		}
	}
}

