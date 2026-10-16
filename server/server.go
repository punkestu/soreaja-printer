package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

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
	colorPrinter   printer.Service
	thermalPrinter printer.Service
}

// NewServer creates a new instance of the Server with color and thermal printer services.
func NewServer(cfg *config.Config, colorPrinter printer.Service, thermalPrinter printer.Service) *Server {
	return &Server{
		cfg:            cfg,
		colorPrinter:   colorPrinter,
		thermalPrinter: thermalPrinter,
	}
}

// statusResponseWriter intercepts WriteHeader to capture the HTTP response status code for access logging.
type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *statusResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// accessLogMiddleware logs each incoming HTTP request with client IP, method, path, status, and duration.
func accessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		srw := &statusResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK, // default status code
		}

		next.ServeHTTP(srw, r)

		clientIP := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			clientIP = strings.TrimSpace(strings.Split(xff, ",")[0])
		}

		log.Printf("[ACCESS] %s - \"%s %s %s\" %d in %v",
			clientIP,
			r.Method,
			r.URL.Path,
			r.Proto,
			srw.statusCode,
			time.Since(start),
		)
	})
}

// corsMiddleware adds comprehensive CORS headers allowing all origins (including dynamic reflection with credentials),
// methods, headers, private network access (Chrome PNA for local IP access from React apps), and preflight caching.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			// Dynamically reflect the caller's Origin to satisfy browsers when credentials (cookies/auth) are sent
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		} else {
			// Wildcard for non-browser/direct callers
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS, HEAD")

		// Allow any requested headers from React/Axios/Fetch
		if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
			w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
		} else {
			w.Header().Set("Access-Control-Allow-Headers", "*")
		}

		w.Header().Set("Access-Control-Expose-Headers", "*")
		w.Header().Set("Access-Control-Max-Age", "86400")

		// Chrome Private Network Access (PNA):
		// Required when a web app (e.g. React running on http://localhost:3000 or public HTTPS)
		// calls a private IP (e.g. http://10.86.121.245:8080 or http://127.0.0.1:8080).
		if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Routes initializes and returns the HTTP router with access logging and CORS middleware.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/print", s.handlePrint)
	mux.HandleFunc("/api/v1/print/color", s.handleColorPrint)
	mux.HandleFunc("/api/v1/print/thermal", s.handleThermalPrint)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	return accessLogMiddleware(corsMiddleware(mux))
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
