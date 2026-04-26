package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
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

// NewFileLogger creates a logger that writes JSON logs to both the given file and stdout.
// It ensures the parent directory exists. Returns a FileLogger that can be closed.
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

	// Create a multi-writer: file + stdout
	multiWriter := io.MultiWriter(file, os.Stdout)

	// Create JSON handler with the desired log level
	handlerOpts := &slog.HandlerOptions{
		Level: cfg.Level,
	}
	handler := slog.NewJSONHandler(multiWriter, handlerOpts)

	// Return the wrapper containing the logger and the file handle
	return &FileLogger{
		Logger: slog.New(handler),
		file:   file,
	}, nil
}
