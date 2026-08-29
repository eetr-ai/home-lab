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

// clients holds what every Helm action configuration is built from.
//
// The REST config and the discovery client are built once and shared. Discovery
// is dozens of round trips to the API server — every group, every version — and
// rebuilding it per request would make listing releases slow, and noisy in the
// audit log, for information that changes when a CRD is installed and at no other
// time.
type clients struct {
	config    *rest.Config
	discovery discovery.CachedDiscoveryInterface
	mapper    meta.RESTMapper
	logger    *slog.Logger
}

// newClients builds the shared half of the Helm plumbing.
//
// It uses the unbounded REST config. Helm's own operations carry their own
// timeouts and an install that waits for pods legitimately outlasts any single
// API request; a twenty-second ceiling would cut one off mid-way and leave the
// release wedged in a pending state, which is the failure that is hardest to
// recover from.
func newClients(logger *slog.Logger) (*clients, error) {
	config, err := restconfig.NewUnbounded()
	if err != nil {
		return nil, err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("build the discovery client: %w", err)
	}

	cached := memory.NewMemCacheClient(discoveryClient)
	return &clients{
		config:    config,
		discovery: cached,
		mapper:    restmapper.NewDeferredDiscoveryRESTMapper(cached),
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
func (c *clients) configurationFor(namespace string) (*action.Configuration, error) {
	configuration := new(action.Configuration)
	getter := &restClientGetter{clients: c, namespace: namespace}

	if err := configuration.Init(getter, namespace, storageDriver); err != nil {
		return nil, fmt.Errorf("initialise helm for namespace %s: %w", namespace, err)
	}
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
}

func (g *restClientGetter) ToRESTConfig() (*rest.Config, error) {
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
