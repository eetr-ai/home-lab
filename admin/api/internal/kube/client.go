package kube

import (
	"errors"
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// defaultRequestTimeout bounds a single call to the API server. Kept under the
// HTTP server's 30-second write timeout so a stalled read fails as a request.
const defaultRequestTimeout = 20 * time.Second

// NewClientset builds a Kubernetes client for wherever this is running.
//
// In the cluster it uses the pod's own ServiceAccount, which the chart binds to a
// read-only ClusterRole. Off-cluster it falls back to the operator's kubeconfig,
// so the API can be run on a laptop against the same cluster during development —
// with that operator's permissions, not the panel's.
//
// Building a client does not contact the cluster. An unreachable API server costs
// a failed request rather than a process that will not start, the same as the
// database slices.
func NewClientset() (*kubernetes.Clientset, error) {
	config, err := restConfig()
	if err != nil {
		return nil, err
	}

	// Without this a request to an unresponsive API server has no deadline of its
	// own and hangs until the caller gives up. Kept under the HTTP server's write
	// timeout, so a stalled cluster read fails as a request rather than as a
	// response that was never written.
	if config.Timeout == 0 {
		config.Timeout = defaultRequestTimeout
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("build the Kubernetes client: %w", err)
	}
	return clientset, nil
}

// restConfig prefers the in-cluster configuration and falls back to a kubeconfig.
func restConfig() (*rest.Config, error) {
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
