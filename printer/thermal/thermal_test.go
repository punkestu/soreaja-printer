package thermal

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	soreaja "github.com/punkestu/soreaja-thermal"
)

func TestThermalPrinter_DefaultMAC(t *testing.T) {
	_ = soreaja.AlignLeft
	mac := "00:11:22:33:44:55"
	p := NewThermalPrinter(mac)
	if p.DefaultMAC != mac {
		t.Fatalf("Expected DefaultMAC to be %s, got %s", mac, p.DefaultMAC)
	}
}

func createTestImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			if (x+y)%2 == 0 {
				img.Set(x, y, color.Black)
			} else {
				img.Set(x, y, color.White)
			}
		}
	}
	return img
}

func TestThermalPrinter_ValidatePrinterNameError(t *testing.T) {
	p := NewThermalPrinter()
	_, err := p.Print(context.Background(), "invalid;name", "nonexistent.png")
	if err == nil {
		t.Fatal("Expected error for invalid printer name, got nil")
	}
}

func TestThermalPrinter_ImageToESCPOS(t *testing.T) {
	p := NewThermalPrinter()
	img := createTestImage(100, 50)

	payload, err := p.ImageToESCPOS(img)
	if err != nil {
		t.Fatalf("ImageToESCPOS failed: %v", err)
	}

	if len(payload) == 0 {
		t.Fatal("Expected non-empty ESC/POS payload")
	}

	// Verify ESC @ (Init printer: 0x1B, 0x40)
	if payload[0] != 0x1B || payload[1] != 0x40 {
		t.Errorf("Expected initial bytes to be ESC @ (0x1B, 0x40), got [0x%X, 0x%X]", payload[0], payload[1])
	}

	// Verify GS v 0 raster command presence
	expectedRasterHeader := []byte{0x1D, 0x76, 0x30, 0x00}
	if !bytes.Contains(payload, expectedRasterHeader) {
		t.Errorf("Expected payload to contain GS v 0 header (%v)", expectedRasterHeader)
	}
}

func TestThermalPrinter_PrintToFile(t *testing.T) {
	p := NewThermalPrinter()

	// Create test PNG file
	tempImg, err := os.CreateTemp("", "test-thermal-*.png")
	if err != nil {
		t.Fatalf("Failed to create temp image: %v", err)
	}
	defer os.Remove(tempImg.Name())

	if err := png.Encode(tempImg, createTestImage(80, 40)); err != nil {
		t.Fatalf("Failed to encode test PNG: %v", err)
	}
	tempImg.Close()

	// Create mock output destination file
	outDest, err := os.CreateTemp("", "mock-thermal-dest-*")
	if err != nil {
		t.Fatalf("Failed to create mock dest file: %v", err)
	}
	defer os.Remove(outDest.Name())
	outDest.Close()

	jobID, err := p.Print(context.Background(), outDest.Name(), tempImg.Name())
	if err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	if jobID == "" {
		t.Error("Expected non-empty jobID")
	}

	// Ensure bytes were actually written to output file
	content, err := os.ReadFile(outDest.Name())
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}
	if len(content) == 0 {
		t.Error("Expected output destination file to have written bytes")
	}
}
