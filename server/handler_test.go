package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
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

func buildMultipartRequest(fieldName, fileName string, fileContent []byte, printerName string) (*http.Request, error) {
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

	req := httptest.NewRequest(http.MethodPost, "/api/v1/print", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}

func TestPrintEndpoint_Success(t *testing.T) {
	cfg := &config.Config{
		ServerPort:         "8080",
		DefaultPrinterName: "EPSON_L120_Series",
		MaxUploadSize:      10 << 20,
		PrintTimeout:       5 * time.Second,
	}

	var capturedPrinter string
	var capturedFilePath string

	mock := &mockPrinterService{
		printFn: func(ctx context.Context, printerName string, filePath string) (string, error) {
			capturedPrinter = printerName
			capturedFilePath = filePath

			// Ensure file exists during printing
			if _, err := os.Stat(filePath); err != nil {
				return "", errors.New("temp file does not exist")
			}
			return "EPSON_L120_Series-999", nil
		},
	}

	srv := NewServer(cfg, mock)
	router := srv.Routes()

	pngBytes := createTestPNG()
	req, err := buildMultipartRequest("image", "test.png", pngBytes, "custom_printer")
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Status != "success" {
		t.Errorf("Expected status 'success', got %s", resp.Status)
	}
	if resp.JobID != "EPSON_L120_Series-999" {
		t.Errorf("Expected job_id 'EPSON_L120_Series-999', got %s", resp.JobID)
	}
	if capturedPrinter != "custom_printer" {
		t.Errorf("Expected target printer 'custom_printer', got %s", capturedPrinter)
	}

	// Verify temp file is removed after request completes
	if _, err := os.Stat(capturedFilePath); !os.IsNotExist(err) {
		t.Errorf("Expected temp file %s to be cleaned up", capturedFilePath)
	}
}

func TestPrintEndpoint_DefaultPrinterFallback(t *testing.T) {
	cfg := &config.Config{
		ServerPort:         "8080",
		DefaultPrinterName: "EPSON_L120_Series",
		MaxUploadSize:      10 << 20,
		PrintTimeout:       5 * time.Second,
	}

	var capturedPrinter string
	mock := &mockPrinterService{
		printFn: func(ctx context.Context, printerName string, filePath string) (string, error) {
			capturedPrinter = printerName
			return "job-1", nil
		},
	}

	srv := NewServer(cfg, mock)
	req, err := buildMultipartRequest("image", "sample.jpg", createTestJPEG(), "")
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	if capturedPrinter != "EPSON_L120_Series" {
		t.Errorf("Expected fallback to EPSON_L120_Series, got %s", capturedPrinter)
	}
}

func TestPrintEndpoint_InvalidFileFormat(t *testing.T) {
	cfg := config.Load()
	mock := &mockPrinterService{}
	srv := NewServer(cfg, mock)

	// Send plain text instead of image
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
	srv := NewServer(cfg, mock)

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

func TestPrintEndpoint_MissingImage(t *testing.T) {
	cfg := config.Load()
	mock := &mockPrinterService{}
	srv := NewServer(cfg, mock)

	// Missing image field
	req, err := buildMultipartRequest("wrong_field", "sample.png", createTestPNG(), "")
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400 for missing image, got %d", w.Code)
	}
}

func TestHealthEndpoint(t *testing.T) {
	cfg := config.Load()
	srv := NewServer(cfg, &mockPrinterService{})

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
	srv := NewServer(cfg, &mockPrinterService{})
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
}
