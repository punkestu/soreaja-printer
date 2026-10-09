package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"soreaja-printer/config"
	"soreaja-printer/printer"
	"soreaja-printer/server"
)

func main() {
	cfg := config.Load()
	printerService := printer.NewCUPSPrinter()
	srv := server.NewServer(cfg, printerService)

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
		log.Printf("Default printer configured: %s", cfg.DefaultPrinterName)
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
