package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"soreaja-printer/config"
)

type mockPrinterService struct {
	printFn func(ctx context.Context, printerName string, filePath string) (string, error)
}

func (m *mockPrinterService) Print(ctx context.Context, printerName string, filePath string) (string, error) {
	if m.printFn != nil {
		return m.printFn(ctx, printerName, filePath)
	}
	return "mock-job-id-123", nil
}

func createTestPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func createTestJPEG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			img.Set(x, y, color.RGBA{B: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

func buildMultipartRequestTo(targetURL, fieldName, fileName string, fileContent []byte, printerName string) (*http.Request, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if fileContent != nil {
		part, err := writer.CreateFormFile(fieldName, fileName)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(part, bytes.NewReader(fileContent)); err != nil {
			return nil, err
		}
	}

	if printerName != "" {
		if err := writer.WriteField("printer", printerName); err != nil {
			return nil, err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	req := httptest.NewRequest(http.MethodPost, targetURL, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}

func buildMultipartRequest(fieldName, fileName string, fileContent []byte, printerName string) (*http.Request, error) {
	return buildMultipartRequestTo("/api/v1/print", fieldName, fileName, fileContent, printerName)
}

func TestPrintEndpoint_EmptyDefaultsToThermal(t *testing.T) {
	cfg := &config.Config{
		ServerPort:                "8080",
		DefaultColorPrinterName:   "EPSON_L120_Series",
		DefaultThermalPrinterName: "POS58_Thermal_Printer",
		MaxUploadSize:             10 << 20,
		PrintTimeout:              5 * time.Second,
	}

	var thermalCalled bool
	var capturedPrinter string

	mockThermal := &mockPrinterService{
		printFn: func(ctx context.Context, printerName string, filePath string) (string, error) {
			thermalCalled = true
			capturedPrinter = printerName
			return "thermal-job-1", nil
		},
	}
	mockColor := &mockPrinterService{
		printFn: func(ctx context.Context, printerName string, filePath string) (string, error) {
			t.Fatal("Color printer should NOT be called when printer is empty")
			return "", nil
		},
	}

	srv := NewServer(cfg, mockColor, mockThermal)
	req, err := buildMultipartRequest("image", "sample.png", createTestPNG(), "")
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	if !thermalCalled {
		t.Error("Expected thermal printer to be called")
	}
	if capturedPrinter != "POS58_Thermal_Printer" {
		t.Errorf("Expected fallback to POS58_Thermal_Printer, got %s", capturedPrinter)
	}
}

func TestPrintEndpoint_ThermalNameRoutesToThermal(t *testing.T) {
	cfg := &config.Config{
		ServerPort:                "8080",
		DefaultColorPrinterName:   "EPSON_L120_Series",
		DefaultThermalPrinterName: "POS58_Thermal_Printer",
		MaxUploadSize:             10 << 20,
		PrintTimeout:              5 * time.Second,
	}

	var thermalCalled bool
	var capturedPrinter string

	mockThermal := &mockPrinterService{
		printFn: func(ctx context.Context, printerName string, filePath string) (string, error) {
			thermalCalled = true
			capturedPrinter = printerName
			return "thermal-job-2", nil
		},
	}
	mockColor := &mockPrinterService{}

	srv := NewServer(cfg, mockColor, mockThermal)
	req, err := buildMultipartRequest("image", "sample.jpg", createTestJPEG(), "thermal")
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	if !thermalCalled {
		t.Error("Expected thermal printer to be called when printer='thermal'")
	}
	if capturedPrinter != "POS58_Thermal_Printer" {
		t.Errorf("Expected fallback to POS58_Thermal_Printer, got %s", capturedPrinter)
	}
}

func TestPrintEndpoint_ElseRoutesToColor(t *testing.T) {
	cfg := &config.Config{
		ServerPort:                "8080",
		DefaultColorPrinterName:   "EPSON_L120_Series",
		DefaultThermalPrinterName: "POS58_Thermal_Printer",
		MaxUploadSize:             10 << 20,
		PrintTimeout:              5 * time.Second,
	}

	var colorCalled bool
	var capturedPrinter string

	mockColor := &mockPrinterService{
		printFn: func(ctx context.Context, printerName string, filePath string) (string, error) {
			colorCalled = true
			capturedPrinter = printerName
			return "color-job-1", nil
		},
	}
	mockThermal := &mockPrinterService{
		printFn: func(ctx context.Context, printerName string, filePath string) (string, error) {
			t.Fatal("Thermal printer should NOT be called for other printer names")
			return "", nil
		},
	}

	srv := NewServer(cfg, mockColor, mockThermal)
	req, err := buildMultipartRequest("image", "sample.png", createTestPNG(), "EPSON_L120_Series")
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	if !colorCalled {
		t.Error("Expected color printer to be called")
	}
	if capturedPrinter != "EPSON_L120_Series" {
		t.Errorf("Expected target printer 'EPSON_L120_Series', got %s", capturedPrinter)
	}
}

func TestPrintEndpoint_ColorKeywordRoutesToDefaultColor(t *testing.T) {
	cfg := &config.Config{
		ServerPort:                "8080",
		DefaultColorPrinterName:   "EPSON_L120_Series",
		DefaultThermalPrinterName: "POS58_Thermal_Printer",
		MaxUploadSize:             10 << 20,
		PrintTimeout:              5 * time.Second,
	}

	var capturedPrinter string
	mockColor := &mockPrinterService{
		printFn: func(ctx context.Context, printerName string, filePath string) (string, error) {
			capturedPrinter = printerName
			return "color-job-2", nil
		},
	}

	srv := NewServer(cfg, mockColor, &mockPrinterService{})
	req, err := buildMultipartRequest("image", "sample.png", createTestPNG(), "color")
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	if capturedPrinter != "EPSON_L120_Series" {
		t.Errorf("Expected default color printer 'EPSON_L120_Series', got %s", capturedPrinter)
	}
}

func TestPrintEndpoint_MissingImage(t *testing.T) {
	cfg := config.Load()
	mock := &mockPrinterService{}
	srv := NewServer(cfg, mock, mock)

	// Missing image field (sending wrong field)
	req, err := buildMultipartRequest("not_image", "sample.png", createTestPNG(), "")
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400 for missing image, got %d", w.Code)
	}
}

func TestPrintEndpoint_InvalidFileFormat(t *testing.T) {
	cfg := config.Load()
	mock := &mockPrinterService{}
	srv := NewServer(cfg, mock, mock)

	// Send plain text instead of JPEG/PNG image
	req, err := buildMultipartRequest("image", "bad.txt", []byte("Hello plain text not image!"), "")
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400 for bad format, got %d", w.Code)
	}

	var resp Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Status != "error" {
		t.Errorf("Expected error response status, got %s", resp.Status)
	}
}

func TestPrintEndpoint_InvalidPrinterName(t *testing.T) {
	cfg := config.Load()
	mock := &mockPrinterService{}
	srv := NewServer(cfg, mock, mock)

	req, err := buildMultipartRequest("image", "sample.png", createTestPNG(), "bad_printer; rm -rf")
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400 for malicious printer name, got %d", w.Code)
	}
}

func TestHealthEndpoint(t *testing.T) {
	cfg := config.Load()
	srv := NewServer(cfg, &mockPrinterService{}, &mockPrinterService{})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var resp Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Status != "ok" {
		t.Errorf("Expected status 'ok', got %s", resp.Status)
	}
}

func TestCORS_HeadersAndPreflight(t *testing.T) {
	cfg := config.Load()
	srv := NewServer(cfg, &mockPrinterService{}, &mockPrinterService{})
	router := srv.Routes()

	// 1. Preflight OPTIONS request
	reqOptions := httptest.NewRequest(http.MethodOptions, "/api/v1/print", nil)
	wOptions := httptest.NewRecorder()
	router.ServeHTTP(wOptions, reqOptions)

	if wOptions.Code != http.StatusNoContent {
		t.Fatalf("Expected status 204 for OPTIONS preflight, got %d", wOptions.Code)
	}

	if origin := wOptions.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Errorf("Expected Access-Control-Allow-Origin '*', got %q", origin)
	}
	if methods := wOptions.Header().Get("Access-Control-Allow-Methods"); methods == "" {
		t.Errorf("Expected Access-Control-Allow-Methods header to be set")
	}
	if headers := wOptions.Header().Get("Access-Control-Allow-Headers"); headers != "*" {
		t.Errorf("Expected Access-Control-Allow-Headers '*', got %q", headers)
	}

	// 2. Normal GET request has CORS headers
	reqGet := httptest.NewRequest(http.MethodGet, "/health", nil)
	wGet := httptest.NewRecorder()
	router.ServeHTTP(wGet, reqGet)

	if origin := wGet.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Errorf("Expected Access-Control-Allow-Origin '*' on GET, got %q", origin)
	}

	// 3. React App / Non-HTTPS preflight with credentials and Private Network Access
	reqReact := httptest.NewRequest(http.MethodOptions, "/api/v1/print", nil)
	reqReact.Header.Set("Origin", "http://localhost:3000")
	reqReact.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization, X-Requested-With")
	reqReact.Header.Set("Access-Control-Request-Private-Network", "true")

	wReact := httptest.NewRecorder()
	router.ServeHTTP(wReact, reqReact)

	if wReact.Code != http.StatusNoContent {
		t.Fatalf("Expected status 204 for React preflight, got %d", wReact.Code)
	}
	if origin := wReact.Header().Get("Access-Control-Allow-Origin"); origin != "http://localhost:3000" {
		t.Errorf("Expected reflected origin 'http://localhost:3000', got %q", origin)
	}
	if creds := wReact.Header().Get("Access-Control-Allow-Credentials"); creds != "true" {
		t.Errorf("Expected Access-Control-Allow-Credentials 'true', got %q", creds)
	}
	if pna := wReact.Header().Get("Access-Control-Allow-Private-Network"); pna != "true" {
		t.Errorf("Expected Access-Control-Allow-Private-Network 'true', got %q", pna)
	}
	if headers := wReact.Header().Get("Access-Control-Allow-Headers"); headers != "Content-Type, Authorization, X-Requested-With" {
		t.Errorf("Expected reflected allowed headers, got %q", headers)
	}
}

func TestPrintEndpoint_MACAddressRoutesToThermal(t *testing.T) {
	cfg := &config.Config{
		ServerPort:                "8080",
		DefaultColorPrinterName:   "EPSON_L120_Series",
		DefaultThermalPrinterName: "POS58_Thermal_Printer",
		DefaultThermalPrinterMAC:  "00:11:22:33:44:55",
		MaxUploadSize:             10 << 20,
		PrintTimeout:              5 * time.Second,
	}

	var thermalCalled bool
	var capturedPrinter string

	mockThermal := &mockPrinterService{
		printFn: func(ctx context.Context, printerName string, filePath string) (string, error) {
			thermalCalled = true
			capturedPrinter = printerName
			return "thermal-mac-job-1", nil
		},
	}
	mockColor := &mockPrinterService{}

	srv := NewServer(cfg, mockColor, mockThermal)
	req, err := buildMultipartRequest("image", "sample.png", createTestPNG(), "86:67:7A:7C:0A:56")
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	if !thermalCalled {
		t.Error("Expected thermal printer to be called for MAC address")
	}
	if capturedPrinter != "86:67:7A:7C:0A:56" {
		t.Errorf("Expected printer target '86:67:7A:7C:0A:56', got %s", capturedPrinter)
	}
}

func TestPrintEndpoint_DefaultThermalPrinterMACFallback(t *testing.T) {
	cfg := &config.Config{
		ServerPort:                "8080",
		DefaultColorPrinterName:   "EPSON_L120_Series",
		DefaultThermalPrinterName: "POS58_Thermal_Printer",
		DefaultThermalPrinterMAC:  "00:11:22:33:44:55",
		MaxUploadSize:             10 << 20,
		PrintTimeout:              5 * time.Second,
	}

	var capturedPrinter string

	mockThermal := &mockPrinterService{
		printFn: func(ctx context.Context, printerName string, filePath string) (string, error) {
			capturedPrinter = printerName
			return "thermal-mac-job-2", nil
		},
	}

	srv := NewServer(cfg, &mockPrinterService{}, mockThermal)
	req, err := buildMultipartRequest("image", "sample.png", createTestPNG(), "")
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	if capturedPrinter != "00:11:22:33:44:55" {
		t.Errorf("Expected default MAC '00:11:22:33:44:55', got %s", capturedPrinter)
	}
}
