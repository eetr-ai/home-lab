package helm

import (
	"fmt"
	"log/slog"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/registry"

	"github.com/eetr-ai/home-lab/admin/api/internal/restconfig"
)

// storageDriver is where Helm keeps a release.
//
// Secrets, in the namespace the release was installed into. Not configmaps,
// which are deprecated and which anything able to list them can read; not sql,
// which is a database this design deliberately does not have; not memory, which
// would lose every release on a restart and would give the API's two replicas
// different answers.
//
// Choosing Secrets is what makes Helm's own storage the source of truth, and it
// is also the reason this slice needs to read Secrets at all. See the deploy Role
// in charts/admin/templates/api/rbac-deploy.yaml.
const storageDriver = "secret"

// operationKind says whether a Helm action is one that must finish inside a
// request or one that is allowed to take minutes.
type operationKind int

const (
	// forReading is served on the request path and is bounded.
	forReading operationKind = iota
	// forWriting runs off the request path, under the job's own timeout.
	forWriting
)

// clients holds what every Helm action configuration is built from.
//
// The REST config and the discovery client are built once and shared. Discovery
// is dozens of round trips to the API server — every group, every version — and
// rebuilding it per request would make listing releases slow, and noisy in the
// audit log, for information that changes when a CRD is installed and at no other
// time.
type clients struct {
	// config bounds a request, and is what every read uses. Reads are served on
	// the request path: a listing that never returns holds a goroutine and a
	// connection to the API server, and Helm's read actions take no context, so
	// the caller hanging up does not end one.
	config *rest.Config
	// unbounded has no deadline, and is only for the operations that legitimately
	// outlast a request. An install waits for pods to come up; a twenty-second
	// ceiling would cut it off mid-way and leave the release wedged, which is the
	// state that is hardest to recover from. What bounds those instead is the
	// job's own timeout.
	unbounded *rest.Config
	discovery discovery.CachedDiscoveryInterface
	mapper    meta.RESTMapper
	// registry pulls charts from an OCI registry. Built once and shared: it holds
	// a credentials store and an HTTP client, and Helm refuses an OCI reference
	// outright when the configuration carries none — "missing registry client",
	// which is what an install of an OCI-hosted chart fails with if this is not
	// wired through.
	registry *registry.Client
	logger   *slog.Logger
}

// newClients builds the shared half of the Helm plumbing.
//
// Two REST configurations, because reads and writes want opposite things. A read
// is served inside a request and must not outlive one; an install waits for pods
// and legitimately does. Helm's actions take no context, so a caller hanging up
// cannot end either — the deadline is the only thing that can, which is why the
// read path gets one.
func newClients(logger *slog.Logger) (*clients, error) {
	config, err := restconfig.New()
	if err != nil {
		return nil, err
	}

	unbounded, err := restconfig.NewUnbounded()
	if err != nil {
		return nil, err
	}

	// Discovery is a read, so it takes the bounded configuration.
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("build the discovery client: %w", err)
	}

	// One registry client for the process. Building it reads the OCI credentials
	// file if there is one and contacts nothing.
	registryClient, err := registry.NewClient()
	if err != nil {
		return nil, fmt.Errorf("build the OCI registry client: %w", err)
	}

	cached := memory.NewMemCacheClient(discoveryClient)
	return &clients{
		config:    config,
		unbounded: unbounded,
		discovery: cached,
		mapper:    restmapper.NewDeferredDiscoveryRESTMapper(cached),
		registry:  registryClient,
		logger:    logger,
	}, nil
}

// configurationFor builds a Helm action configuration bound to one namespace.
//
// Cheap, because everything expensive is shared: Init stores the clients and
// opens the storage driver, and does not talk to the cluster. A per-request
// configuration therefore costs a struct rather than a round trip.
//
// Bound to one namespace rather than to the cluster, and that is the whole
// design: reading releases across every namespace would need a cluster-wide grant
// on Secrets. Instead each managed namespace is asked separately, with a Role
// that reaches only into it.
//
// The kind picks which REST configuration the actions get: a read is bounded, a
// write is not. Asking the caller to say which is deliberate — there is no way to
// infer it here, and defaulting either way would silently give one of them the
// wrong deadline.
func (c *clients) configurationFor(namespace string, kind operationKind) (*action.Configuration, error) {
	configuration := new(action.Configuration)
	getter := &restClientGetter{clients: c, namespace: namespace, kind: kind}

	if err := configuration.Init(getter, namespace, storageDriver); err != nil {
		return nil, fmt.Errorf("initialise helm for namespace %s: %w", namespace, err)
	}

	// Set before any action is built from this configuration: NewInstall and
	// NewUpgrade copy it at construction, so assigning it afterwards is assigning
	// it to nothing. Without it, LocateChart refuses every oci:// reference with
	// "missing registry client" — and this lab publishes its own admin chart to
	// an OCI registry, so that is not a hypothetical path.
	configuration.RegistryClient = c.registry
	return configuration, nil
}

// restClientGetter is what Helm asks for a cluster connection.
//
// genericclioptions.ConfigFlags, the implementation Helm's CLI uses, reads a
// kubeconfig from disk — and in the cluster there is not one. This supplies the
// same four answers from the in-cluster configuration instead, sharing the
// discovery client and REST mapper across every namespace so only the namespace
// itself differs per call.
type restClientGetter struct {
	clients   *clients
	namespace string
	kind      operationKind
}

func (g *restClientGetter) ToRESTConfig() (*rest.Config, error) {
	if g.kind == forWriting {
		return g.clients.unbounded, nil
	}
	return g.clients.config, nil
}

// The next three return interfaces because RESTClientGetter says they do. The
// shape of this type is Helm's to decide, not this package's.
//
//nolint:ireturn // the signature is fixed by genericclioptions.RESTClientGetter
func (g *restClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return g.clients.discovery, nil
}

//nolint:ireturn // the signature is fixed by genericclioptions.RESTClientGetter
func (g *restClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	return g.clients.mapper, nil
}

// ToRawKubeConfigLoader returns a client config whose namespace is the one this
// getter is bound to.
//
// Helm calls this and reads the namespace off it. Returning an error here, or a
// loader that resolves to "default", makes every action operate on the wrong
// namespace — which for a read is an empty list and for a write is far worse.
//
//nolint:ireturn // the signature is fixed by genericclioptions.RESTClientGetter
func (g *restClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	overrides := &clientcmd.ConfigOverrides{Context: clientcmdapi.Context{Namespace: g.namespace}}
	return clientcmd.NewDefaultClientConfig(*clientcmdapi.NewConfig(), overrides)
}

// invalidateDiscovery drops the cached view of what kinds the cluster serves.
//
// Called before an install or an upgrade, because a chart that creates a
// CustomResourceDefinition and then an instance of it needs the mapper to resolve
// a kind that did not exist when the cache was filled. Not called before a read:
// a stale cache costs nothing there, and refilling it is dozens of requests.
func (c *clients) invalidateDiscovery() {
	c.discovery.Invalidate()
}
