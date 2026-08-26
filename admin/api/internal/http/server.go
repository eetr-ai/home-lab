package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const (
	// Bounds on a single request, so a stalled client cannot hold a connection
	// open indefinitely. Generous enough that no legitimate call approaches them.
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second

	// How long in-flight requests get to finish after a shutdown signal. Kept
	// under the ten seconds Kubernetes allows by default, so the process exits on
	// its own terms rather than being killed partway through.
	shutdownTimeout = 8 * time.Second
)

// Serve runs an HTTP server until ctx is cancelled, then drains it.
//
// Returning the error that ended it — rather than logging and exiting here — is
// what keeps the process's exit path in one place, in main.
func Serve(ctx context.Context, addr string, handler http.Handler, logger *slog.Logger) error {
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	failed := make(chan error, 1)
	go func() {
		logger.Info("listening", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			failed <- fmt.Errorf("listen on %s: %w", server.Addr, err)
			return
		}
		failed <- nil
	}()

	select {
	case err := <-failed:
		return err
	case <-ctx.Done():
		logger.Info("draining")
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("drain the server: %w", err)
	}
	return nil
}
