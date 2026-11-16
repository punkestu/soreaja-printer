package gui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"soreaja-printer/config"
	"soreaja-printer/server"
)

type mockPrinter struct {
	mu       sync.Mutex
	lastPath string
	lastDest string
	calls    int
}

func (m *mockPrinter) Print(ctx context.Context, printerName string, filePath string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.lastDest = printerName
	m.lastPath = filePath
	return fmt.Sprintf("mock-job-%d", m.calls), nil
}

func TestLogStream(t *testing.T) {
	ls := NewLogStream(5)

	var received []string
	var mu sync.Mutex
	ls.RegisterListener(func(line string) {
		mu.Lock()
		received = append(received, line)
		mu.Unlock()
	})

	for i := 1; i <= 7; i++ {
		_, err := ls.Write([]byte(fmt.Sprintf("Log entry %d\n", i)))
		if err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}
	}

	// Buffer limit is 5
	lines := ls.Lines()
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines in buffer, got %d", len(lines))
	}
	if lines[0] != "Log entry 3" {
		t.Errorf("expected first line to be 'Log entry 3', got '%s'", lines[0])
	}
	if lines[4] != "Log entry 7" {
		t.Errorf("expected last line to be 'Log entry 7', got '%s'", lines[4])
	}

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if len(received) != 7 {
		t.Errorf("expected listener to receive 7 entries, got %d", len(received))
	}
	mu.Unlock()

	ls.Clear()
	if len(ls.Lines()) != 0 {
		t.Errorf("expected 0 lines after Clear(), got %d", len(ls.Lines()))
	}
}

func TestServerManager_Lifecycle(t *testing.T) {
	cfg := &config.Config{
		ServerPort:                "18080",
		DefaultColorPrinterName:   "EPSON_L120_Series",
		DefaultThermalPrinterName: "/dev/cu.RPP02N",
		MaxUploadSize:             10 << 20,
		PrintTimeout:              5 * time.Second,
	}

	colorMock := &mockPrinter{}
	thermalMock := &mockPrinter{}

	sm := NewServerManager(cfg, colorMock, thermalMock)
	if sm.IsRunning() {
		t.Fatal("expected server to not be running initially")
	}

	statusCh := make(chan bool, 5)
	sm.OnStatusChange(func(running bool, msg string) {
		statusCh <- running
	})

	// Start server
	err := sm.Start("18080")
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	select {
	case running := <-statusCh:
		if !running {
			t.Errorf("expected running=true notification")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for start notification")
	}

	if !sm.IsRunning() {
		t.Error("expected sm.IsRunning() == true")
	}

	// Verify handler responds to /health
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	srv := server.NewServer(cfg, colorMock, thermalMock)
	srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	// Stop server
	err = sm.Stop()
	if err != nil {
		t.Fatalf("failed to stop server: %v", err)
	}

	select {
	case running := <-statusCh:
		if running {
			t.Errorf("expected running=false notification")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stop notification")
	}

	if sm.IsRunning() {
		t.Error("expected sm.IsRunning() == false after stop")
	}
}

func TestPrinterTester_GeneratorsAndTestPrint(t *testing.T) {
	colorMock := &mockPrinter{}
	thermalMock := &mockPrinter{}
	tester := NewPrinterTester(colorMock, thermalMock)

	// Verify test images
	colorImg := GenerateColorTestImage("EPSON_L120_Series")
	if colorImg.Bounds().Dx() != 500 || colorImg.Bounds().Dy() != 350 {
		t.Errorf("expected color image 500x350, got %dx%d", colorImg.Bounds().Dx(), colorImg.Bounds().Dy())
	}

	thermalImg := GenerateThermalTestImage("86:67:7A:7C:0A:56")
	if thermalImg.Bounds().Dx() != 384 || thermalImg.Bounds().Dy() != 320 {
		t.Errorf("expected thermal image 384x320, got %dx%d", thermalImg.Bounds().Dx(), thermalImg.Bounds().Dy())
	}

	ctx := context.Background()

	// Test Color print
	jobID, err := tester.TestColorPrint(ctx, "EPSON_L120_Series")
	if err != nil {
		t.Fatalf("unexpected error during TestColorPrint: %v", err)
	}
	if jobID != "mock-job-1" {
		t.Errorf("expected mock-job-1, got %s", jobID)
	}

	// Test Thermal print
	jobID, err = tester.TestThermalPrint(ctx, "86:67:7A:7C:0A:56")
	if err != nil {
		t.Fatalf("unexpected error during TestThermalPrint: %v", err)
	}
	if jobID != "mock-job-1" {
		t.Errorf("expected mock-job-1, got %s", jobID)
	}

	// Thermal target validation
	status, err := tester.CheckThermalStatus(ctx, "86:67:7A:7C:0A:56")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status == "" {
		t.Errorf("expected non-empty status for valid MAC")
	}
}

