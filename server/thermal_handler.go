package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"soreaja-printer/printer"
)

func (s *Server) handleThermalPrint(w http.ResponseWriter, r *http.Request) {
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
	printerName := strings.TrimSpace(r.FormValue("printer"))
	if printerName == "" || strings.EqualFold(printerName, "thermal") {
		if s.cfg.DefaultThermalPrinterMAC != "" {
			printerName = s.cfg.DefaultThermalPrinterMAC
		} else {
			printerName = s.cfg.DefaultThermalPrinterName
		}
	}

	if err := printer.ValidatePrinterName(printerName); err != nil {
		log.Printf("[WARN] Invalid printer name provided: '%s'", printerName)
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 3. Extract uploaded file (accepts "image" or "file")
	file, fileHeader, err := r.FormFile("image")
	if err != nil {
		file, fileHeader, err = r.FormFile("file")
	}
	if err != nil {
		log.Printf("[WARN] Missing 'image' or 'file' field in form data: %v", err)
		writeJSONError(w, http.StatusBadRequest, "Missing required 'image' or 'file' in multipart form")
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
	case "application/pdf":
		fileExt = ".pdf"
	case "text/plain; charset=utf-8", "text/plain":
		fileExt = ".txt"
	default:
		log.Printf("[WARN] Unsupported content type detected for thermal print: %s (filename: %s)", contentType, fileHeader.Filename)
		writeJSONError(w, http.StatusBadRequest, "Invalid file format: only JPEG, PNG, PDF, or text files are supported for thermal printing")
		return
	}

	// Rewind file pointer after reading the header
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		log.Printf("[ERROR] Failed to seek file: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to process uploaded file")
		return
	}

	// 5. Create secure temporary file
	tempFile, err := os.CreateTemp("", fmt.Sprintf("thermal-print-*%s", fileExt))
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
		writeJSONError(w, http.StatusInternalServerError, "Failed to write temporary print data")
		return
	}

	// Close temporary file before passing path to lp command
	if err := tempFile.Close(); err != nil {
		log.Printf("[ERROR] Failed to close temp file: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to finalize temporary print file")
		return
	}

	// 6. Execute print command with timeout context
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.PrintTimeout)
	defer cancel()

	log.Printf("[INFO] Sending job for %s to thermal printer '%s'", filepath.Base(tempFilePath), printerName)
	jobID, err := s.thermalPrinter.Print(ctx, printerName, tempFilePath)
	if err != nil {
		log.Printf("[ERROR] Thermal print execution failed for printer '%s': %v", printerName, err)
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Print job failed: %v", err))
		return
	}

	// 7. Success response
	log.Printf("[INFO] Thermal print job successfully submitted: %s", jobID)
	writeJSONResponse(w, http.StatusOK, Response{
		Status:  "success",
		Message: "Job sent to thermal printer successfully",
		JobID:   jobID,
	})
}
