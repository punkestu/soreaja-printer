package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"soreaja-printer/config"
	"soreaja-printer/printer/color"
	"soreaja-printer/printer/thermal"
	"soreaja-printer/server"
)

func getLocalIPv4s() []string {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
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
				ips = append(ips, ip.String())
			}
		}
	}
	return ips
}

func main() {
	cfg := config.Load()
	colorPrinterService := color.NewColorPrinter()
	thermalPrinterService := thermal.NewThermalPrinter(cfg.DefaultThermalPrinterMAC)
	srv := server.NewServer(cfg, colorPrinterService, thermalPrinterService)

	httpServer := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      srv.Routes(),
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Channel to listen for interrupt or termination signals
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Starting soreaja-printer microservice on port :%s", cfg.ServerPort)
		log.Printf("  -> Local:   http://localhost:%s", cfg.ServerPort)
		for _, ip := range getLocalIPv4s() {
			log.Printf("  -> Network: http://%s:%s", ip, cfg.ServerPort)
		}
		log.Printf("Default color printer: %s", cfg.DefaultColorPrinterName)
		log.Printf("Default thermal printer: %s", cfg.DefaultThermalPrinterName)
		if cfg.DefaultThermalPrinterMAC != "" {
			log.Printf("Default thermal printer MAC: %s", cfg.DefaultThermalPrinterMAC)
		}
		log.Printf("Max upload payload size: %d bytes (~%.2f MB)", cfg.MaxUploadSize, float64(cfg.MaxUploadSize)/(1024*1024))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	sig := <-shutdownCh
	log.Printf("Received signal %s; shutting down gracefully...", sig)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Graceful shutdown error: %v", err)
		if closeErr := httpServer.Close(); closeErr != nil {
			log.Printf("Forced close error: %v", closeErr)
		}
	}

	fmt.Println("Server shut down successfully.")
}
