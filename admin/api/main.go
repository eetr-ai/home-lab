// Command admin-api is the home lab's administration API.
//
// It manages the PostgreSQL and MongoDB services running on the virtualization
// host and reads the Kubernetes cluster, and it publishes an OpenAPI description
// of itself so a caller can look up what it offers rather than being told.
//
// This first iteration is deliberately minimal: it serves a readiness endpoint
// and nothing else, so the build, release, and deployment path can be proven
// before there is anything to break.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	defaultPort = "8090"

	// Bounds on a single request, so a stalled client cannot hold a connection
	// open indefinitely. Generous enough that no legitimate call approaches them.
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second

	// How long in-flight requests get to finish after a shutdown signal. Kept
	// under the ten seconds Kubernetes allows by default, so the process exits on
	// its own terms rather than being killed.
	shutdownTimeout = 8 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("server stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

// run owns the server lifecycle and returns the error that ended it, so main
// stays a thin entry point and every exit path flows through one place.
func run(logger *slog.Logger) error {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	// Listen for the signals a container runtime sends, so a rollout drains
	// rather than dropping whatever was in flight.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 1)
	go func() {
		logger.Info("listening", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
			return
		}
		errs <- nil
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	return server.Shutdown(shutdownCtx) //nolint:wrapcheck // the caller logs it as-is
}

// healthz answers whether the process is up. It deliberately checks nothing else:
// a readiness probe that fails when PostgreSQL is unreachable would take the
// panel out of service exactly when an operator needs it to say so.
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
