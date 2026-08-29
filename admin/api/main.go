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
	"fmt"
	"log/slog"
	stdhttp "net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/eetr-ai/home-lab/admin/api/internal/auth"
	"github.com/eetr-ai/home-lab/admin/api/internal/health"
	"github.com/eetr-ai/home-lab/admin/api/internal/helm"
	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
	"github.com/eetr-ai/home-lab/admin/api/internal/kube"
	"github.com/eetr-ai/home-lab/admin/api/internal/mongo"
	"github.com/eetr-ai/home-lab/admin/api/internal/nspolicy"
	"github.com/eetr-ai/home-lab/admin/api/internal/openapi"
	"github.com/eetr-ai/home-lab/admin/api/internal/postgres"
)

const (
	defaultPort = "8090"

	// How a boolean is spelled in this process's environment. One spelling, so a
	// switch cannot be on in one place and off in another because someone wrote
	// "TRUE".
	envTrue = "true"

	// Discovery is one HTTP call to the identity provider at startup. Bounded so
	// an unreachable provider fails the process quickly and visibly, rather than
	// leaving it hanging with no logs and no listener.
	discoveryTimeout = 15 * time.Second

	// How long one Helm operation may take when ADMIN_HELM_TIMEOUT says nothing.
	// Long, because it covers pulling a chart, applying it, and waiting for the
	// pods to become ready.
	defaultHelmTimeout = 10 * time.Minute
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
	// What a caller may do, as opposed to whether it is a caller at all. Off by
	// default: the panel's own token names no scopes today, and requiring them
	// before the identity provider issues them would lock the panel out of its
	// own API. See auth.NewGuard.
	requireScopes := os.Getenv("ADMIN_OIDC_REQUIRE_SCOPES") == envTrue
	if requireScopes {
		logger.Info("refusing any token that names no scopes")
	}
	guard := auth.NewGuard(requireScopes)

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

	policy := namespacePolicy(logger)

	if err := registerKubernetes(api, guard, policy, logger); err != nil {
		return err
	}

	if err := registerHelm(ctx, api, guard, policy, logger); err != nil {
		return err
	}

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

// registerKubernetes wires the cluster slice unless it is switched off.
//
// Unlike the databases this needs no connection string: in the cluster the pod's
// own ServiceAccount is the credential, and off-cluster a kubeconfig is. It is on
// by default for that reason, and ADMIN_KUBERNETES_DISABLED exists for running the
// API somewhere with neither.
func registerKubernetes(mux *stdhttp.ServeMux, guard *auth.Guard, policy nspolicy.Policy,
	logger *slog.Logger,
) error {
	if os.Getenv("ADMIN_KUBERNETES_DISABLED") == envTrue {
		logger.Warn("ADMIN_KUBERNETES_DISABLED is set; the cluster endpoints are not served")
		return nil
	}

	clientset, err := kube.NewClientset()
	if err != nil {
		return err
	}

	// A second client with no request deadline, for log streaming only. The one
	// above bounds every request at twenty seconds, which is right for a list and
	// fatal for a follow stream.
	streamClient, err := kube.NewStreamClientset()
	if err != nil {
		return err
	}

	// The metrics client is built unconditionally and used only if it answers.
	// metrics-server is an optional cluster component, so its absence has to be a
	// missing reading on the dashboard rather than a panel that will not start.
	metrics, err := kube.NewMetricsClientset()
	if err != nil {
		return err
	}

	// Node disk usage comes from the kubelet rather than from the Kubernetes API,
	// which means a grant on the nodes/proxy subresource. That also opens the
	// kubelet's other read endpoints, so it is opt-in: the chart withholds the
	// grant unless this is switched on, and without it the panel reports every
	// other node figure and no disk.
	nodeStats := os.Getenv("ADMIN_KUBERNETES_NODE_STATS") == envTrue
	if nodeStats {
		logger.Info("reading node disk usage from the kubelet")
	}

	repo := kube.NewRepository(clientset, streamClient, metrics, nodeStats)
	service := kube.NewService(repo, policy, os.Getenv("ADMIN_NAMESPACE_POD_SECURITY"))
	kube.NewHandler(service, guard).Register(mux)
	logger.Info("serving the Kubernetes endpoints")
	return nil
}

// registerHelm wires the Helm slice unless it is switched off.
//
// Like the cluster slice this needs no connection string: Helm reads its releases
// through the same in-cluster credential, out of Secrets in the namespaces this
// lab named. It is off unless a namespace was named — with none, every route
// answers 501, which is the honest reply for a capability that was built and not
// switched on.
//
// Switching it on is not free, and the chart says so at length: reading a release
// means reading Secrets in that namespace, and RBAC cannot narrow that to Helm's
// own.
func registerHelm(ctx context.Context, mux *stdhttp.ServeMux, guard *auth.Guard,
	policy nspolicy.Policy, logger *slog.Logger,
) error {
	if os.Getenv("ADMIN_HELM_DISABLED") == envTrue {
		logger.Warn("ADMIN_HELM_DISABLED is set; the Helm endpoints are not served")
		return nil
	}

	timeout, err := helmTimeout()
	if err != nil {
		return err
	}

	repo, err := helm.NewRepository(logger, timeout)
	if err != nil {
		return err
	}

	// Declared as the interface and assigned only when there is one, because a
	// nil *helm.Store handed to an interface parameter is an interface that is
	// not nil — and the service would then call through it.
	var deployments helm.DeploymentStore
	if store := helmStore(ctx, logger); store != nil {
		deployments = store
	}

	helm.NewHandler(helm.NewService(repo, deployments, policy, timeout, logger), guard).
		Register(mux)
	logger.Info("serving the Helm endpoints",
		slog.Any("namespaces", policy.ManagedNamespaces()),
		slog.Bool("everyNamespace", policy.ManagesEverything()),
		slog.Duration("timeout", timeout))
	return nil
}

// helmStore opens the record of what this lab has declared, and returns nil when
// there is no record to open.
//
// Nil is a supported state, not a failure: without a connection string the
// release endpoints still read the cluster and only the deployment endpoints
// answer 501. A database that is configured and unreachable is also not fatal —
// it is logged and the deployment endpoints answer 503 — because an API that
// refuses to start because PostgreSQL is down takes with it the pages that would
// have said so.
//
// The consequence to be honest about: a store that fails here is not retried, so
// the deployment endpoints stay down until the pod restarts. Restarting on a
// schedule is what Kubernetes already does for a pod whose probes fail, and
// wiring a retry loop in here would be a worse version of it.
func helmStore(ctx context.Context, logger *slog.Logger) *helm.Store {
	dsn := os.Getenv("ADMIN_HELM_DSN")
	if dsn == "" {
		logger.Warn("ADMIN_HELM_DSN is unset; Helm deployments cannot be declared, " +
			"and the release endpoints still read the cluster")
		return nil
	}

	store, err := helm.NewStore(ctx, dsn, logger)
	if err != nil {
		logger.Error("the Helm deployment store is unavailable; those endpoints will fail",
			slog.Any("error", err))
		return nil
	}
	logger.Info("recording Helm deployments in PostgreSQL")
	return store
}

// helmTimeout bounds one install, upgrade, rollback, or uninstall.
//
// Generous by default, because it is off the request path entirely: the caller
// was answered with a 202 long before. What it protects against is an operation
// that never finishes holding a release in a pending state forever.
func helmTimeout() (time.Duration, error) {
	value := os.Getenv("ADMIN_HELM_TIMEOUT")
	if value == "" {
		return defaultHelmTimeout, nil
	}

	timeout, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("ADMIN_HELM_TIMEOUT is not a duration: %w", err)
	}
	if timeout <= 0 {
		return 0, fmt.Errorf("ADMIN_HELM_TIMEOUT must be positive, and is %s", timeout)
	}
	return timeout, nil
}

// namespacePolicy reads which namespaces this lab will not let the panel touch.
//
// POD_NAMESPACE comes from the downward API rather than from a constant, because
// the chart can be installed under any release name and the one namespace that
// must always be protected is the one this process is running in. Missing it is
// survivable — the built-in rules and the configured list still apply — but it
// means the panel could be asked to delete itself, so it is said loudly.
func namespacePolicy(logger *slog.Logger) nspolicy.Policy {
	own := os.Getenv("POD_NAMESPACE")
	if own == "" {
		logger.Warn("POD_NAMESPACE is unset; the panel's own namespace is not protected")
	}

	return nspolicy.New(nspolicy.Config{
		Own:       own,
		Protected: splitList(os.Getenv("ADMIN_PROTECTED_NAMESPACES")),
		Managed:   splitList(os.Getenv("ADMIN_HELM_MANAGED_NAMESPACES")),
		// Every unprotected namespace, rather than a named list. Set by the chart
		// when the lab has chosen the cluster-scoped grant, and meaningless
		// without it: the policy would permit a namespace the ServiceAccount
		// still cannot read.
		ManageEverything: os.Getenv("ADMIN_HELM_ALL_NAMESPACES") == envTrue,
	})
}

// splitList reads a comma-separated environment value, ignoring the empty
// entries a trailing comma or a templated-in blank leaves behind.
func splitList(value string) []string {
	var items []string
	for _, item := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
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
