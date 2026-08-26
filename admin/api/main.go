// Command admin-api is the home lab's administration API.
//
// It manages the PostgreSQL and MongoDB services running on the virtualization
// host and reads the Kubernetes cluster, and it publishes an OpenAPI description
// of itself so a caller can look up what it offers rather than being told.
//
// This file is wiring and nothing else. Every behavior lives in a slice under
// internal/, folded by component rather than by layer — see
// docs/contributing/layer-conventions.md.
package main

import (
	"context"
	"log/slog"
	stdhttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eetr-ai/home-lab/admin/api/internal/auth"
	"github.com/eetr-ai/home-lab/admin/api/internal/health"
	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
	"github.com/eetr-ai/home-lab/admin/api/internal/mongo"
	"github.com/eetr-ai/home-lab/admin/api/internal/openapi"
	"github.com/eetr-ai/home-lab/admin/api/internal/postgres"
)

const (
	defaultPort = "8090"

	// Discovery is one HTTP call to the identity provider at startup. Bounded so
	// an unreachable provider fails the process quickly and visibly, rather than
	// leaving it hanging with no logs and no listener.
	discoveryTimeout = 15 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("admin-api stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	// Signals a container runtime sends, so a rollout drains rather than dropping
	// whatever was in flight.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	verifier, err := newVerifier(ctx, logger)
	if err != nil {
		return err
	}

	// Routes under /api require a verified caller. Everything else does not, and
	// the two are separate muxes so that is a property of the wiring rather than
	// something each handler has to remember.
	api := stdhttp.NewServeMux()
	auth.NewHandler().Register(api)

	// Each managed service is registered only when it is configured. A panel with
	// no PostgreSQL to administer answers 404 for those routes, which is the
	// honest reply — and the OpenAPI description still lists them, so a caller can
	// see the capability exists and is not switched on here.
	closePostgres, err := registerPostgres(ctx, api, logger)
	if err != nil {
		return err
	}
	defer closePostgres()

	closeMongo, err := registerMongo(api, logger)
	if err != nil {
		return err
	}
	defer closeMongo(ctx)

	root := stdhttp.NewServeMux()
	health.New().Register(root)
	openapi.New().Register(root)
	root.Handle("/api/", auth.Middleware(verifier)(api))

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	return httpx.Serve(ctx, ":"+port, root, logger)
}

// registerPostgres wires the PostgreSQL slice when a connection string is set.
//
// Configuring it does not connect: the pool is lazy, so an unreachable database
// costs a failed request rather than a process that will not start. The panel's
// other sections keep working while it is down, which is when an operator most
// wants to look at them.
func registerPostgres(ctx context.Context, mux *stdhttp.ServeMux, logger *slog.Logger) (func(), error) {
	dsn := os.Getenv("ADMIN_POSTGRES_DSN")
	if dsn == "" {
		logger.Warn("ADMIN_POSTGRES_DSN is unset; the PostgreSQL endpoints are not served")
		return func() {}, nil
	}

	repo, err := postgres.NewRepository(ctx, dsn)
	if err != nil {
		return nil, err
	}
	postgres.NewHandler(postgres.NewService(repo)).Register(mux)
	logger.Info("serving the PostgreSQL endpoints")
	return repo.Close, nil
}

// registerMongo wires the MongoDB slice when a connection string is set.
//
// Like PostgreSQL, configuring it does not connect: the driver dials lazily, so
// an unreachable server costs a failed request rather than a panel that will not
// start.
func registerMongo(mux *stdhttp.ServeMux, logger *slog.Logger) (func(context.Context), error) {
	uri := os.Getenv("ADMIN_MONGO_URI")
	if uri == "" {
		logger.Warn("ADMIN_MONGO_URI is unset; the MongoDB endpoints are not served")
		return func(context.Context) {}, nil
	}

	repo, err := mongo.NewRepository(uri)
	if err != nil {
		return nil, err
	}
	mongo.NewHandler(mongo.NewService(repo)).Register(mux)
	logger.Info("serving the MongoDB endpoints")
	return repo.Close, nil
}

// newVerifier builds the token verifier from the environment.
//
// A missing issuer is fatal rather than a mode. This API is a resource server:
// running it without one would leave every endpoint it grows open to anyone who
// can reach the pod, and "authentication was not configured" is not a state worth
// being able to reach by accident.
func newVerifier(ctx context.Context, logger *slog.Logger) (*auth.OIDCVerifier, error) {
	issuer := os.Getenv("ADMIN_OIDC_ISSUER")
	audience := os.Getenv("ADMIN_OIDC_AUDIENCE")

	if audience == "" {
		// Survivable, and worth saying loudly: without it any token this issuer
		// signed for any application is accepted here.
		logger.Warn("ADMIN_OIDC_AUDIENCE is unset; tokens will not be checked against an audience")
	}

	discoveryCtx, cancel := context.WithTimeout(ctx, discoveryTimeout)
	defer cancel()

	verifier, err := auth.NewOIDCVerifier(discoveryCtx, issuer, audience)
	if err != nil {
		return nil, err
	}
	logger.Info("verifying bearer tokens", slog.String("issuer", issuer), slog.String("audience", audience))
	return verifier, nil
}
