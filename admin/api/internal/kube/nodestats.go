package kube

import (
	"context"
	"encoding/json"
	"log/slog"
)

// statsSummary is the part of the kubelet's /stats/summary this reads.
//
// Only the node's root filesystem. The full document also carries per-pod and
// per-container statistics, and decoding what is not used would tie this to a
// kubelet payload that changes between versions for no benefit.
type statsSummary struct {
	Node struct {
		Fs struct {
			CapacityBytes  *int64 `json:"capacityBytes"`
			UsedBytes      *int64 `json:"usedBytes"`
			AvailableBytes *int64 `json:"availableBytes"`
		} `json:"fs"`
	} `json:"node"`
}

// nodeFilesystem reads one node's root disk from its kubelet.
//
// Off unless switched on, because there is no other source: node disk usage is
// not in the Kubernetes API at all, and metrics-server reports only CPU and
// memory. Reading it means a `get` on the nodes/proxy subresource, which also
// permits the kubelet's other read endpoints — so it is an explicit choice rather
// than a grant the panel holds by default. See charts/admin/templates/api/rbac.yaml.
//
// Failure is not an error: a kubelet that refuses or is unreachable costs this one
// figure, and the rest of the node's report is still worth showing.
func (r *Repository) nodeFilesystem(ctx context.Context, node string) (Filesystem, bool) {
	if !r.nodeStats {
		return Filesystem{}, false
	}

	raw, err := r.client.CoreV1().RESTClient().Get().
		Resource("nodes").Name(node).SubResource("proxy").
		Suffix("stats", "summary").
		DoRaw(ctx)
	if err != nil {
		slog.Warn("could not read node stats from the kubelet",
			slog.String("node", node), slog.Any("error", err))
		return Filesystem{}, false
	}

	var summary statsSummary
	if err := json.Unmarshal(raw, &summary); err != nil {
		slog.Warn("could not parse the kubelet's stats summary",
			slog.String("node", node), slog.Any("error", err))
		return Filesystem{}, false
	}

	filesystem := summary.Node.Fs
	// Every field is a pointer in the kubelet's own schema, and a summary with no
	// capacity is one that measured nothing rather than a disk of size zero.
	if filesystem.CapacityBytes == nil {
		return Filesystem{}, false
	}
	return Filesystem{
		CapacityBytes:  *filesystem.CapacityBytes,
		UsedBytes:      derefInt64(filesystem.UsedBytes),
		AvailableBytes: derefInt64(filesystem.AvailableBytes),
	}, true
}

// logMetricsUnavailable reports a metrics API that did not answer.
//
// At warning rather than error: on a cluster without metrics-server this is the
// steady state, and logging it as a failure would make every dashboard load look
// like a fault.
func (r *Repository) logMetricsUnavailable(err error, what string) {
	slog.Warn("the metrics API did not answer; usage will be reported as unavailable",
		slog.String("read", what), slog.Any("error", err))
}

func derefInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
