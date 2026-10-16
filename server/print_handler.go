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

	// 2. Resolve printer target and routing logic:
	// - If printer string is empty -> default to thermal
	// - If printer name is "thermal" -> print with thermal
	// - Else -> use color printer
	printerParam := strings.TrimSpace(r.FormValue("printer"))

	var (
		targetPrinter  string
		useThermal     bool
		targetService  printer.Service
		printerTypeStr string
	)

	if printerParam == "" || strings.EqualFold(printerParam, "thermal") {
		useThermal = true
		if s.cfg.DefaultThermalPrinterMAC != "" {
			targetPrinter = s.cfg.DefaultThermalPrinterMAC
		} else {
			targetPrinter = s.cfg.DefaultThermalPrinterName
		}
		targetService = s.thermalPrinter
		printerTypeStr = "thermal"
	} else if printer.IsMACAddress(printerParam) {
		useThermal = true
		targetPrinter = printerParam
		targetService = s.thermalPrinter
		printerTypeStr = "thermal"
	} else {
		useThermal = false
		if strings.EqualFold(printerParam, "color") {
			targetPrinter = s.cfg.DefaultColorPrinterName
		} else {
			targetPrinter = printerParam
		}
		targetService = s.colorPrinter
		printerTypeStr = "color"
	}

	if err := printer.ValidatePrinterName(targetPrinter); err != nil {
		log.Printf("[WARN] Invalid printer name provided: '%s'", targetPrinter)
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
	prefix := "color-print-*"
	if useThermal {
		prefix = "thermal-print-*"
	}
	tempFile, err := os.CreateTemp("", fmt.Sprintf("%s%s", prefix, fileExt))
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

	log.Printf("[INFO] Sending job for %s to %s printer '%s'", filepath.Base(tempFilePath), printerTypeStr, targetPrinter)
	jobID, err := targetService.Print(ctx, targetPrinter, tempFilePath)
	if err != nil {
		log.Printf("[ERROR] %s print execution failed for printer '%s': %v", printerTypeStr, targetPrinter, err)
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Print job failed: %v", err))
		return
	}

	// 7. Success response
	log.Printf("[INFO] Print job successfully submitted to %s printer: %s", printerTypeStr, jobID)
	writeJSONResponse(w, http.StatusOK, Response{
		Status:  "success",
		Message: "Job sent to printer successfully",
		JobID:   jobID,
	})
}
