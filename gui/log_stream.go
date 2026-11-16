package gui

import (
	"bytes"
	"strings"
	"sync"
)

// LogStream is a thread-safe in-memory log buffer that implements io.Writer
// and broadcasts log lines to registered listeners.
type LogStream struct {
	mu        sync.RWMutex
	lines     []string
	maxLines  int
	listeners []func(string)
}

// NewLogStream creates a new LogStream holding up to maxLines in memory.
func NewLogStream(maxLines int) *LogStream {
	if maxLines <= 0 {
		maxLines = 1000
	}
	return &LogStream{
		lines:    make([]string, 0, maxLines),
		maxLines: maxLines,
	}
}

// Write implements io.Writer. It splits incoming byte chunks into lines,
// stores them, and notifies all registered listeners.
func (ls *LogStream) Write(p []byte) (n int, err error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	raw := string(bytes.TrimRight(p, "\r\n"))
	if raw == "" {
		return len(p), nil
	}

	splitLines := strings.Split(raw, "\n")
	for _, line := range splitLines {
		cleanLine := strings.TrimRight(line, "\r")
		if cleanLine == "" {
			continue
		}

		if len(ls.lines) >= ls.maxLines {
			// Shift out oldest line
			ls.lines = ls.lines[1:]
		}
		ls.lines = append(ls.lines, cleanLine)

		for _, listener := range ls.listeners {
			if listener != nil {
				go listener(cleanLine)
			}
		}
	}

	return len(p), nil
}

// RegisterListener adds a callback invoked whenever a new log line is written.
func (ls *LogStream) RegisterListener(cb func(string)) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.listeners = append(ls.listeners, cb)
}

// Lines returns a snapshot of all buffered log lines.
func (ls *LogStream) Lines() []string {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	copied := make([]string, len(ls.lines))
	copy(copied, ls.lines)
	return copied
}

// String returns all buffered lines joined by newlines.
func (ls *LogStream) String() string {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return strings.Join(ls.lines, "\n")
}

// Clear flushes all buffered lines.
func (ls *LogStream) Clear() {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.lines = ls.lines[:0]
}

