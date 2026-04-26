// Package serverutil provides utilities for creating and running HTTP servers with graceful shutdown.
package serverutil

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"suppa-ahg-stack/common-golang/logger"
)

// Options configures the behaviour of ServerUtil.
type Options struct {
	// Addr is the address:port to listen on (e.g. "localhost:8080").
	// If empty, the server will attempt to read WEB_SERVER_ADDRESS and
	// WEB_SERVER_PORT from environment variables.
	Addr string

	// ShutdownTimeout is the maximum duration to wait for pending requests
	// to finish during graceful shutdown. Defaults to 10 seconds if zero.
	ShutdownTimeout time.Duration

	// Handler is the http.Handler to use. It can be set later via CreateServer,
	// or you can pass it directly to CreateServer. The field is optional.
	Handler http.Handler

	// Logger is the structured logger. If nil, slog.Default() is used.
	Logger *logger.FileLogger
}

// ServerUtil holds the configuration and provides methods to create and run an HTTP server.
type ServerUtil struct {
	opts Options
}

// NewServerUtil creates a new ServerUtil with the given options.
// If Options.Addr is empty, environment variables will be used.
// If Options.ShutdownTimeout is zero, 10 seconds will be used.
// If Options.Logger is nil, slog.Default() will be used.
func NewServerUtil(opts Options) (*ServerUtil, error) {
	if opts.ShutdownTimeout == 0 {
		opts.ShutdownTimeout = 10 * time.Second
	}
	if opts.Logger == nil {
		return nil, errors.New("logger should be included in the option struct")
	}
	return &ServerUtil{opts: opts}, nil
}

// CreateServer builds an http.Server using the configured options.
// If the handler parameter is non-nil, it overrides Options.Handler.
// Returns an error if the address cannot be resolved.
func (su *ServerUtil) CreateServer(handler http.Handler) (*http.Server, error) {
	addr := su.opts.Addr

	h := su.opts.Handler
	if handler != nil {
		h = handler
	}
	if h == nil {
		return nil, errors.New("no handler provided")
	}

	return &http.Server{
		Addr:    addr,
		Handler: h,
	}, nil
}

// RunServer starts the server and handles graceful shutdown.
// It listens for interrupt signals and context cancellation.
// The provided *http.Server is typically created by CreateServer.
func (su *ServerUtil) RunServer(ctx context.Context, srv *http.Server) error {
	if srv == nil {
		return errors.New("http.Server is nil")
	}

	serveError := make(chan error, 1)
	go func() {
		su.logf("Starting server on %s", srv.Addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serveError <- err
		}
		close(serveError)
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	var err error
	select {
	case err = <-serveError:
		// Server failed to start or crashed
		return err
	case <-stop:
		su.logf("Shutdown signal received")
	case <-ctx.Done():
		su.logf("Context cancelled")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), su.opts.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		// Force close if shutdown fails
		if closeErr := srv.Close(); closeErr != nil {
			return errors.Join(err, closeErr)
		}
		return err
	}

	su.logf("Server exited gracefully")
	return nil
}

// logf writes a formatted log message at Info level using the configured slog.Logger.
func (su *ServerUtil) logf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	su.opts.Logger.Info(msg)
}
