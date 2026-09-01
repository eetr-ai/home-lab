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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	stdhttp "net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/eetr-ai/home-lab/admin/api/internal/auth"
	"github.com/eetr-ai/home-lab/admin/api/internal/health"
	"github.com/eetr-ai/home-lab/admin/api/internal/helm"
	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
	"github.com/eetr-ai/home-lab/admin/api/internal/kube"
	"github.com/eetr-ai/home-lab/admin/api/internal/mongo"
	"github.com/eetr-ai/home-lab/admin/api/internal/nsenrol"
	"github.com/eetr-ai/home-lab/admin/api/internal/nspolicy"
	"github.com/eetr-ai/home-lab/admin/api/internal/openapi"
	"github.com/eetr-ai/home-lab/admin/api/internal/postgres"
	"github.com/eetr-ai/home-lab/admin/api/internal/secretgen"
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
)

// main serves the API, unless it was asked to perform one Helm operation.
//
// The subcommand is not a mode of the server. It is a different program that
// happens to share this binary, and it shares it deliberately: chart resolution,
// values merging, and the rollout stamp then have exactly one implementation, and
// the Job the API creates runs this same image.
//
// One switch on one argument rather than a command framework. There are two
// commands, and cobra — already here indirectly, through Helm — would be a
// dependency and a worldview in exchange for parsing that.
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if len(os.Args) > 1 && os.Args[1] == helm.RunCommand {
		// The exit code is the answer. It becomes the pod's, which becomes the
		// Job's status, which is what the panel and a pipeline read — so a Helm
		// failure that exited zero would be reported as a successful deploy.
		if err := helm.RunJob(context.Background(), os.Args[2:], logger); err != nil {
			logger.Error("the helm operation failed", slog.Any("error", err))
			os.Exit(1)
		}
		return
	}

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
	// Always registered: it has nothing to configure and nothing to connect to,
	// so there is no installation where it would be honest to answer 404.
	secretgen.NewHandler().Register(api)

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

	// Built once and handed to both slices, because it is one answer: the cluster
	// slice enrols a namespace and reports whether each is set up, and the Helm
	// slice asks which ones it may work in. Two copies would be two answers.
	enrol, err := namespaceEnrolment(policy, logger)
	if err != nil {
		return err
	}

	if err := registerKubernetes(api, policy, enrol, logger); err != nil {
		return err
	}

	if err := registerHelm(ctx, api, policy, enrol, logger); err != nil {
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
func registerKubernetes(mux *stdhttp.ServeMux, policy nspolicy.Policy,
	enrol *nsenrol.Service, logger *slog.Logger,
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
	nodeStats := os.Getenv("ADMIN_KUBERNETES_NODE_STATS") == "true"
	if nodeStats {
		logger.Info("reading node disk usage from the kubelet")
	}

	repo := kube.NewRepository(clientset, streamClient, metrics, nodeStats)

	// Declared as the interface and assigned only when there is one, the same way
	// the Helm slice's dependencies are: a typed nil handed to an interface
	// parameter is an interface that is not nil, and every enrolment answer would
	// then be a call through it instead of an absent one.
	var enrolment kube.Enrolment
	if enrol != nil {
		enrolment = enrol
	}

	service, err := kube.NewService(repo, policy, os.Getenv("ADMIN_NAMESPACE_POD_SECURITY"),
		enrolment)
	if err != nil {
		return err
	}
	kube.NewHandler(service).Register(mux)
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
func registerHelm(ctx context.Context, mux *stdhttp.ServeMux, policy nspolicy.Policy,
	enrol *nsenrol.Service, logger *slog.Logger,
) error {
	if os.Getenv("ADMIN_HELM_DISABLED") == envTrue {
		logger.Warn("ADMIN_HELM_DISABLED is set; the Helm endpoints are not served")
		return nil
	}

	timeout, err := helm.Timeout()
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

	// Only when this lab can enrol a namespace at all. Without that every mutation
	// answers 501 before it reaches a Job, so requiring the Job's configuration
	// would fail a panel that was only ever meant to read releases — and the read
	// routes work without any of it.
	//
	// Note what is NOT decided here any more: which namespaces are deployable.
	// That used to come from an environment variable, so enrolling one meant
	// restarting these pods; it is read from the cluster now, per request.
	var runner *helm.JobRepository
	if enrol != nil {
		jobConfig, err := helmJobConfig(timeout)
		if err != nil {
			return err
		}
		if runner, err = helm.NewJobRepository(jobConfig); err != nil {
			return err
		}
	}

	// Declared as the interface and assigned only when there is one, for the same
	// reason the store is: a nil *helm.JobRepository handed to an interface
	// parameter is an interface that is not nil, and the service would call
	// through it instead of answering 501.
	var operations helm.Jobs
	if runner != nil {
		operations = runner
	}

	// The same nil-interface trap as the cluster slice: assigned only when there
	// is one, so an unconfigured panel answers 501 instead of calling through a
	// typed nil.
	var enrolment helm.Enrolment
	if enrol != nil {
		enrolment = enrol
	}

	helm.NewHandler(helm.NewService(repo, deployments, operations, enrolment, policy, timeout, logger)).
		Register(mux)
	logger.Info("serving the Helm endpoints",
		slog.Bool("enrolment", enrol != nil),
		slog.Duration("timeout", timeout))
	return nil
}

// namespaceEnrolment builds the service that enrols namespaces as Helm targets,
// or reports that this lab does not deploy from the panel.
//
// The ClusterRoles it binds are rendered by the chart, so the release name is
// what says whether they exist. Absent, nothing here can bind anything and every
// route that would answers 501 — which is the honest reply for a capability that
// is built and not switched on.
//
// Every field comes from this process's own environment. Nothing a caller sends
// reaches it, which is what keeps "which grant does enrolling hand out" a
// property of the chart rather than of a request.
func namespaceEnrolment(policy nspolicy.Policy, logger *slog.Logger) (*nsenrol.Service, error) {
	if os.Getenv("ADMIN_KUBERNETES_DISABLED") == envTrue {
		return nil, nil
	}

	config := nsenrol.Config{
		Release:    os.Getenv("ADMIN_HELM_RELEASE_NAME"),
		Namespace:  os.Getenv("POD_NAMESPACE"),
		APIAccount: os.Getenv("ADMIN_API_SERVICE_ACCOUNT"),
		JobAccount: os.Getenv("ADMIN_HELM_JOB_SERVICE_ACCOUNT"),
	}
	if config.Release == "" {
		logger.Warn("ADMIN_HELM_RELEASE_NAME is unset; namespaces cannot be enrolled for Helm")
		return nil, nil
	}
	if !config.Valid() {
		// Said out loud at startup rather than discovered on the first enrolment:
		// a panel configured to deploy and unable to name the objects it would
		// create is a misconfiguration, not a runtime condition.
		return nil, fmt.Errorf(
			"helm enrolment is configured but incomplete: POD_NAMESPACE, " +
				"ADMIN_API_SERVICE_ACCOUNT and ADMIN_HELM_JOB_SERVICE_ACCOUNT must all be set")
	}

	clientset, err := kube.NewClientset()
	if err != nil {
		return nil, err
	}
	logger.Info("namespaces can be enrolled as Helm targets", slog.String("release", config.Release))
	return nsenrol.NewService(nsenrol.NewRepository(clientset), config, policy), nil
}

// helmJobConfig describes the Job that performs one Helm operation.
//
// Every field is read here, from this process's own environment, and none of it
// is ever influenced by a request. That is the point rather than a detail: the
// account these Jobs run as holds the whole deploy grant, so a request able to
// name one would be a request able to name any account in this namespace.
//
// It is read at startup rather than per operation so a malformed value fails the
// pod immediately. Deferring it would surface a typo in the chart as a 500 on the
// first deploy, which might be weeks later.
func helmJobConfig(timeout time.Duration) (helm.JobConfig, error) {
	config := helm.JobConfig{
		Namespace:       os.Getenv("POD_NAMESPACE"),
		Image:           os.Getenv("ADMIN_HELM_JOB_IMAGE"),
		ImagePullPolicy: corev1.PullPolicy(os.Getenv("ADMIN_HELM_JOB_IMAGE_PULL_POLICY")),
		PullSecrets:     splitList(os.Getenv("ADMIN_HELM_JOB_PULL_SECRETS")),
		ServiceAccount:  os.Getenv("ADMIN_HELM_JOB_SERVICE_ACCOUNT"),
		Timeout:         timeout,
		// The name and the key, never the value. The API holds its own connection
		// string in its environment and must not copy it into a Job: a Job is not
		// a Secret, and anything able to list Jobs here would be able to read it.
		DSNSecretName: os.Getenv("ADMIN_HELM_DSN_SECRET_NAME"),
		DSNSecretKey:  os.Getenv("ADMIN_HELM_DSN_SECRET_KEY"),
	}

	if config.Namespace == "" {
		return helm.JobConfig{}, errors.New(
			"POD_NAMESPACE is unset, and it is the namespace Helm jobs are created in")
	}
	if config.Image == "" {
		return helm.JobConfig{}, errors.New(
			"ADMIN_HELM_JOB_IMAGE is unset, and a Helm job runs this same image")
	}
	if config.ServiceAccount == "" {
		return helm.JobConfig{}, errors.New(
			"ADMIN_HELM_JOB_SERVICE_ACCOUNT is unset, and a Helm job needs the deploy grant")
	}

	// Parsed at 32 bits because that is what the field is. Atoi would accept a
	// value this then silently truncated — a TTL of 2^31+60 becoming 60 seconds
	// is a job whose log is gone a minute after it finishes, which would look
	// like the cluster reaping early rather than like a typo in a values file.
	ttl, err := strconv.ParseInt(os.Getenv("ADMIN_HELM_JOB_TTL_SECONDS"), 10, 32)
	if err != nil {
		return helm.JobConfig{}, fmt.Errorf(
			"ADMIN_HELM_JOB_TTL_SECONDS is not a number of seconds that fits in 32 bits: %w", err)
	}
	if ttl < 0 {
		return helm.JobConfig{}, fmt.Errorf(
			"ADMIN_HELM_JOB_TTL_SECONDS must not be negative, and is %d", ttl)
	}
	config.TTLSeconds = int32(ttl)

	// One JSON object rather than four scalars, so the values file keeps the
	// normal Kubernetes shape and there is no second vocabulary to learn.
	if raw := os.Getenv("ADMIN_HELM_JOB_RESOURCES"); raw != "" && raw != "{}" {
		if err := json.Unmarshal([]byte(raw), &config.Resources); err != nil {
			return helm.JobConfig{}, fmt.Errorf("ADMIN_HELM_JOB_RESOURCES is not resource requirements: %w", err)
		}
	}
	return config, nil
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
