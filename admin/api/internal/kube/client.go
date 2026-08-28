package kube

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/eetr-ai/home-lab/admin/api/internal/restconfig"
)

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
	config, err := restconfig.New()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("build the Kubernetes client: %w", err)
	}
	return clientset, nil
}

// NewMetricsClientset builds a client for the metrics API, when one is wanted.
//
// Separate from the main clientset because metrics.k8s.io is served by an
// optional aggregated API rather than by the API server itself. Building this
// contacts nothing, so it succeeds on a cluster with no metrics-server; the
// absence surfaces as a failed read, which the repository degrades to "no
// reading" rather than to an error.
func NewMetricsClientset() (*metricsclient.Clientset, error) {
	config, err := restconfig.New()
	if err != nil {
		return nil, err
	}

	clientset, err := metricsclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("build the Kubernetes metrics client: %w", err)
	}
	return clientset, nil
}

// NewStreamClientset builds a second client for long-lived reads.
//
// The clientset NewClientset returns bounds every request, and that bound covers
// reading the response body rather than just getting a reply. A follow stream is
// a response body that never ends, so it would be cut mid-line. See
// restconfig.NewUnbounded for why there is no per-request escape.
//
// What bounds a stream instead is the caller hanging up, which cancels the
// request context, and the handler's own cap on how long one may run.
func NewStreamClientset() (*kubernetes.Clientset, error) {
	config, err := restconfig.NewUnbounded()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("build the Kubernetes log client: %w", err)
	}
	return clientset, nil
}
