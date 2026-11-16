package gui

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"soreaja-printer/config"
	"soreaja-printer/printer"
	"soreaja-printer/server"
)

// ServerManager manages the lifecycle of the in-process HTTP microservice server.
type ServerManager struct {
	mu              sync.Mutex
	running         bool
	activePort      string
	httpServer      *http.Server
	cfg             *config.Config
	colorPrinter    printer.Service
	thermalPrinter  printer.Service
	statusListeners []func(running bool, msg string)
}

// NewServerManager initializes a new ServerManager.
func NewServerManager(cfg *config.Config, colorPrinter printer.Service, thermalPrinter printer.Service) *ServerManager {
	return &ServerManager{
		cfg:            cfg,
		colorPrinter:   colorPrinter,
		thermalPrinter: thermalPrinter,
		activePort:     cfg.ServerPort,
	}
}

// OnStatusChange registers a callback to be notified when server status changes.
func (sm *ServerManager) OnStatusChange(cb func(running bool, msg string)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.statusListeners = append(sm.statusListeners, cb)
}

func (sm *ServerManager) notifyListeners(running bool, msg string) {
	for _, cb := range sm.statusListeners {
		if cb != nil {
			go cb(running, msg)
		}
	}
}

// IsRunning reports whether the server is currently active.
func (sm *ServerManager) IsRunning() bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.running
}

// ActivePort returns the port the server is running on (or configured to run on).
func (sm *ServerManager) ActivePort() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.activePort
}

// Start boots the HTTP server in a background goroutine on the specified port.
func (sm *ServerManager) Start(port string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.running {
		return errors.New("server is already running")
	}

	port = strings.TrimSpace(port)
	if port == "" {
		port = sm.cfg.ServerPort
	}
	if port == "" {
		port = "8080"
	}

	// Create a shallow copy of config with updated port
	runCfg := *sm.cfg
	runCfg.ServerPort = port

	srv := server.NewServer(&runCfg, sm.colorPrinter, sm.thermalPrinter)

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		sm.notifyListeners(false, fmt.Sprintf("Failed to bind port :%s: %v", port, err))
		return fmt.Errorf("failed to bind port %s: %w", port, err)
	}

	sm.activePort = port
	sm.httpServer = &http.Server{
		Handler:      srv.Routes(),
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	sm.running = true
	sm.notifyListeners(true, fmt.Sprintf("Running on port :%s", port))

	log.Printf("[SERVER] Started soreaja-printer HTTP server on port :%s", port)
	for _, addr := range formatNetworkAddresses(port) {
		log.Printf("  -> %s", addr)
	}

	go func() {
		if err := sm.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[SERVER ERROR] HTTP server terminated unexpectedly: %v", err)
			sm.mu.Lock()
			sm.running = false
			sm.httpServer = nil
			sm.mu.Unlock()
			sm.notifyListeners(false, fmt.Sprintf("Server stopped unexpectedly: %v", err))
		}
	}()

	return nil
}

// Stop gracefully shuts down the running HTTP server.
func (sm *ServerManager) Stop() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.running || sm.httpServer == nil {
		return errors.New("server is not running")
	}

	log.Printf("[SERVER] Stopping HTTP server on port :%s...", sm.activePort)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := sm.httpServer.Shutdown(ctx)
	sm.running = false
	sm.httpServer = nil
	sm.notifyListeners(false, "Server stopped")

	if err != nil {
		log.Printf("[SERVER ERROR] Graceful shutdown error: %v", err)
		return err
	}

	log.Printf("[SERVER] HTTP server stopped successfully.")
	return nil
}

// GetNetworkAddresses returns all accessible HTTP URLs (localhost and LAN IPs).
func (sm *ServerManager) GetNetworkAddresses() []string {
	sm.mu.Lock()
	port := sm.activePort
	sm.mu.Unlock()

	return formatNetworkAddresses(port)
}

func formatNetworkAddresses(port string) []string {
	if port == "" {
		port = "8080"
	}

	urls := []string{fmt.Sprintf("http://localhost:%s", port)}
	ifaces, err := net.Interfaces()
	if err != nil {
		return urls
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && !ip.IsLoopback() && ip.To4() != nil {
				urls = append(urls, fmt.Sprintf("http://%s:%s", ip.String(), port))
			}
		}
	}
	return urls
}

