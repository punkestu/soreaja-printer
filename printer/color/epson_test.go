package color

import (
	"context"
	"testing"
)

func TestColorPrinter_ValidatePrinterNameError(t *testing.T) {
	p := NewColorPrinter()
	_, err := p.Print(context.Background(), "invalid;name", "nonexistent.png")
	if err == nil {
		t.Fatal("Expected error for invalid printer name, got nil")
	}
}
