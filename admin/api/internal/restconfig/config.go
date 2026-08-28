// Package restconfig builds the connection to the Kubernetes API server.
//
// It exists because more than one slice needs that connection and they must not
// reach across the seam to get it: the cluster slice builds three clientsets from
// it, and the Helm slice builds an action configuration from the same thing. A
// second copy of the in-cluster-then-kubeconfig rule would be two places to get
// the fallback wrong, and only one of them would be read.
//
// Building a configuration contacts nothing. An unreachable API server costs a
// failed request rather than a process that will not start.
package restconfig

import (
	"errors"
	"fmt"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// DefaultRequestTimeout bounds a single call to the API server. Kept under the
// HTTP server's 30-second write timeout so a stalled read fails as a request.
const DefaultRequestTimeout = 20 * time.Second

// New returns a configuration whose requests are bounded.
//
// Without a timeout a request to an unresponsive API server has no deadline of
// its own and hangs until the caller gives up. The bound is applied here rather
// than left to each caller, because forgetting it is silent until the day the API
// server stops answering.
//
// A kubeconfig may carry a timeout of its own, and a shorter one is honoured: the
// bound is only imposed when there is none.
func New() (*rest.Config, error) {
	config, err := load()
	if err != nil {
		return nil, err
	}
	if config.Timeout == 0 {
		config.Timeout = DefaultRequestTimeout
	}
	return config, nil
}

// NewUnbounded returns a configuration with no deadline on a request, for the two
// things that legitimately outlast one.
//
// rest.Config.Timeout becomes http.Client.Timeout, which bounds reading the
// response body rather than just getting a reply. A pod log follow is a response
// body that never ends, and a Helm install that waits for pods is a call this
// process makes on its own time; both would be cut mid-way by the bound above.
// There is no per-request escape — rest.Request.Timeout only narrows a deadline,
// it cannot lift one.
//
// What bounds these instead is the caller hanging up, which cancels the request
// context, and each caller's own cap on how long one may run.
//
// The zero is written explicitly rather than left alone: a kubeconfig may carry a
// timeout, and inheriting one here would reintroduce exactly the bug this avoids.
func NewUnbounded() (*rest.Config, error) {
	config, err := load()
	if err != nil {
		return nil, err
	}
	config.Timeout = 0
	return config, nil
}

// load prefers the in-cluster configuration and falls back to a kubeconfig.
//
// In the cluster that is the pod's own ServiceAccount, which the chart binds to
// the roles it declares. Off-cluster it is the operator's kubeconfig, so the API
// can be run on a laptop against the same cluster during development — with that
// operator's permissions, not the panel's.
func load() (*rest.Config, error) {
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}
	// Only "there is no service account here" is a reason to look elsewhere. A
	// malformed in-cluster configuration is a real failure, and silently falling
	// back would hide it behind whatever kubeconfig happened to be on the machine.
	if !errors.Is(err, rest.ErrNotInCluster) {
		return nil, fmt.Errorf("read the in-cluster configuration: %w", err)
	}

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rules, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("read a kubeconfig: %w", err)
	}
	return config, nil
}
