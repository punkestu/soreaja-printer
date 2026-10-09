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
