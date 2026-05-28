package logger

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/lmittmann/tint"
)

// LogConfig holds configuration for the logger.
type LogConfig struct {
	Filename string     // Path to the log file
	Level    slog.Level // Log level (Debug, Info, Warn, Error)
}

// FileLogger wraps the slog.Logger and provides a Close method for the underlying file.
type FileLogger struct {
	*slog.Logger
	file *os.File
}

// Close releases the log file resource.
func (l *FileLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// multiHandler dispatches log records to multiple handlers.
type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}

// NewFileLogger creates a logger that writes JSON logs to the given file
// and colored text logs to stdout. It ensures the parent directory exists.
// Returns a FileLogger that can be closed.
func NewFileLogger(cfg LogConfig) (*FileLogger, error) {
	// Ensure the directory exists
	dir := filepath.Dir(cfg.Filename)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	// Open the log file for appending (create if not exists)
	file, err := os.OpenFile(cfg.Filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}

	// File handler: JSON format for structured parsing
	jsonHandler := slog.NewJSONHandler(file, &slog.HandlerOptions{
		Level: cfg.Level,
	})

	// Console handler: colored text format via tint
	tintHandler := tint.NewHandler(os.Stdout, &tint.Options{
		Level:      cfg.Level,
		TimeFormat: time.DateTime,
	})

	// Combine both handlers
	multi := &multiHandler{
		handlers: []slog.Handler{jsonHandler, tintHandler},
	}

	// Return the wrapper containing the logger and the file handle
	return &FileLogger{
		Logger: slog.New(multi),
		file:   file,
	}, nil
}
