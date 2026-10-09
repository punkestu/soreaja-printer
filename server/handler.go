package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"soreaja-printer/config"
	"soreaja-printer/printer"
)

// Response represents standard API JSON output.
type Response struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	JobID   string `json:"job_id,omitempty"`
}

// Server handles HTTP requests for the print service.
type Server struct {
	cfg            *config.Config
	printerService printer.Service
}

// NewServer creates a new instance of the Server.
func NewServer(cfg *config.Config, printerService printer.Service) *Server {
	return &Server{
		cfg:            cfg,
		printerService: printerService,
	}
}

// Routes initializes and returns the HTTP router.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/print", s.handlePrint)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	writeJSONResponse(w, http.StatusOK, Response{
		Status:  "ok",
		Message: "soreaja-printer service is healthy",
	})
}

func (s *Server) handlePrint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// 1. Enforce payload size limit
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadSize)
	if err := r.ParseMultipartForm(s.cfg.MaxUploadSize); err != nil {
		log.Printf("[ERROR] Parse multipart form failed: %v", err)
		writeJSONError(w, http.StatusBadRequest, "Request payload too large or invalid multipart form")
		return
	}

	// 2. Resolve & sanitize printer destination
	printerName := r.FormValue("printer")
	if printerName == "" {
		printerName = s.cfg.DefaultPrinterName
	}

	if err := printer.ValidatePrinterName(printerName); err != nil {
		log.Printf("[WARN] Invalid printer name provided: '%s'", printerName)
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 3. Extract uploaded image
	file, fileHeader, err := r.FormFile("image")
	if err != nil {
		log.Printf("[WARN] Missing 'image' field in form data: %v", err)
		writeJSONError(w, http.StatusBadRequest, "Missing required 'image' file in multipart form")
		return
	}
	defer file.Close()

	// 4. Sniff MIME type using the first 512 bytes
	headerBytes := make([]byte, 512)
	n, err := file.Read(headerBytes)
	if err != nil && !errors.Is(err, io.EOF) {
		log.Printf("[ERROR] Failed to read uploaded file header: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to inspect uploaded file")
		return
	}

	contentType := http.DetectContentType(headerBytes[:n])
	var fileExt string
	switch contentType {
	case "image/jpeg":
		fileExt = ".jpg"
	case "image/png":
		fileExt = ".png"
	default:
		log.Printf("[WARN] Unsupported content type detected: %s (filename: %s)", contentType, fileHeader.Filename)
		writeJSONError(w, http.StatusBadRequest, "Invalid file format: only JPEG and PNG images are supported")
		return
	}

	// Rewind file pointer after reading the header
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		log.Printf("[ERROR] Failed to seek file: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to process uploaded file")
		return
	}

	// 5. Create secure temporary file
	tempFile, err := os.CreateTemp("", fmt.Sprintf("print-*%s", fileExt))
	if err != nil {
		log.Printf("[ERROR] Failed to create temp file: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to initialize temporary print file")
		return
	}
	tempFilePath := tempFile.Name()

	// Ensure cleanup on return
	defer func() {
		if err := os.Remove(tempFilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("[WARN] Failed to delete temp file %s: %v", tempFilePath, err)
		}
	}()

	// Stream uploaded data to temp file
	if _, err := io.Copy(tempFile, file); err != nil {
		tempFile.Close()
		log.Printf("[ERROR] Failed to write temp file: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to write temporary image data")
		return
	}

	// Close temporary file before passing path to lp command
	if err := tempFile.Close(); err != nil {
		log.Printf("[ERROR] Failed to close temp file: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to finalize temporary image file")
		return
	}

	// 6. Execute print command with timeout context
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.PrintTimeout)
	defer cancel()

	log.Printf("[INFO] Sending job for %s to printer '%s'", filepath.Base(tempFilePath), printerName)
	jobID, err := s.printerService.Print(ctx, printerName, tempFilePath)
	if err != nil {
		log.Printf("[ERROR] Print execution failed for printer '%s': %v", printerName, err)
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Print job failed: %v", err))
		return
	}

	// 7. Success response
	log.Printf("[INFO] Print job successfully submitted: %s", jobID)
	writeJSONResponse(w, http.StatusOK, Response{
		Status:  "success",
		Message: "Job sent to printer successfully",
		JobID:   jobID,
	})
}

func writeJSONError(w http.ResponseWriter, code int, message string) {
	writeJSONResponse(w, code, Response{
		Status:  "error",
		Message: message,
	})
}

func writeJSONResponse(w http.ResponseWriter, code int, resp Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(resp)
}
