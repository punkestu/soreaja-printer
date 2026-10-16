package printer

import (
	"testing"
)

func TestValidatePrinterName(t *testing.T) {
	tests := []struct {
		name        string
		printerName string
		wantErr     bool
	}{
		{"Valid standard name", "EPSON_L120_Series", false},
		{"Valid hyphenated", "epson-l120", false},
		{"Valid simple name", "printer1", false},
		{"Valid serial device", "/dev/cu.RPP02N", false},
		{"Valid usb tty device", "/dev/ttyUSB0", false},
		{"Valid network address", "192.168.1.100:9100", false},
		{"Valid MAC colon", "00:11:22:33:44:55", false},
		{"Valid MAC hyphen", "86-67-7A-7C-0A-56", false},
		{"Invalid MAC length", "00:11:22:33:44", true},
		{"Empty name", "", true},
		{"Command injection semicolon", "printer;rm -rf /", true},
		{"Command injection pipe", "printer|cat", true},
		{"Command injection ampersand", "printer&echo 1", true},
		{"Directory traversal", "../../etc/passwd", true},
		{"Spaces in name", "EPSON L120", true},
		{"Special characters", "printer$1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePrinterName(tt.printerName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePrinterName(%q) error = %v, wantErr = %v", tt.printerName, err, tt.wantErr)
			}
		})
	}
}
