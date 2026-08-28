package kube

import "time"

// Namespace is a namespace in the cluster.
type Namespace struct {
	Name   string    `json:"name"`
	Status string    `json:"status"`
	Age    time.Time `json:"createdAt"`
	// Labels are carried because policy is decided from them — whether the
	// namespace is protected, and whether Helm may write to it — and because the
	// panel shows an operator why a namespace has no delete button.
	Labels map[string]string `json:"labels,omitempty"`
	// Protected reports that this namespace may not be deleted, and why. Decided
	// by internal/nspolicy from the name and the labels above, not by the cluster.
	Protected       bool   `json:"protected"`
	ProtectedReason string `json:"protectedReason,omitempty"`
}

// NamespaceSpec is a request to create a namespace.
//
// Labels are the caller's, and the service adds its own over them; see
// namespaces.go for which, and for the keys a caller may not set.
type NamespaceSpec struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
}

// Workload is a controller that runs pods — a Deployment, StatefulSet, or
// DaemonSet. They are reported as one type because the question an operator asks
// is "what is running here", not "what kind of controller is running here".
type Workload struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	// Desired is how many replicas the controller wants, and Ready how many are
	// serving. A DaemonSet's desired count is per node rather than configured.
	Desired int32 `json:"desired"`
	Ready   int32 `json:"ready"`
	// Images are the container images the pod template runs, which is what says
	// which version is deployed.
	Images    []string  `json:"images"`
	CreatedAt time.Time `json:"createdAt"`
}

// Pod is one pod and the state an operator reads first.
type Pod struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	// Phase is Kubernetes' own phase, and Status is the more useful summary —
	// "Terminating" or a waiting container's reason, which the phase does not say.
	Phase  string `json:"phase"`
	Status string `json:"status"`
	// Ready counts containers, not pods: "1/2" is a running pod that is not
	// serving, which is the case worth seeing.
	Ready    string `json:"ready"`
	Restarts int32  `json:"restarts"`
	Node     string `json:"node"`
	// Containers names each one, which a log request must do when there is more
	// than one. Init containers are included, last.
	Containers []string  `json:"containers"`
	CreatedAt  time.Time `json:"createdAt"`
}

// Event is one cluster event, which is where the reason for a stuck pod lives.
type Event struct {
	Type      string    `json:"type"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Object    string    `json:"object"`
	Count     int32     `json:"count"`
	LastSeen  time.Time `json:"lastSeen"`
	Namespace string    `json:"namespace"`
}

// Resources is a quantity of compute, in the units an operator reads rather than
// the ones Kubernetes stores. CPU is millicores because that is how requests are
// written, and memory is bytes because that is what a size formatter wants.
//
// A zero field means "not reported" rather than "none": a node always has some
// memory, and a metrics reading that omitted it should render as unknown.
type Resources struct {
	CPUMillis      int64 `json:"cpuMillis"`
	MemoryBytes    int64 `json:"memoryBytes"`
	Pods           int64 `json:"pods,omitempty"`
	EphemeralBytes int64 `json:"ephemeralBytes,omitempty"`
}

// Filesystem is a node's root disk, as the kubelet reports it.
//
// Used and Available do not necessarily sum to Capacity: a filesystem reserves
// blocks for root that are neither used nor available to anything else.
type Filesystem struct {
	CapacityBytes  int64 `json:"capacityBytes"`
	UsedBytes      int64 `json:"usedBytes"`
	AvailableBytes int64 `json:"availableBytes"`
}

// Node is one machine in the cluster.
type Node struct {
	Name string `json:"name"`
	// Status is the summary an operator reads first: Ready, NotReady, or
	// Ready,SchedulingDisabled for a cordoned node, which the conditions alone do
	// not say.
	Status string `json:"status"`
	Ready  bool   `json:"ready"`
	// Roles come from the node-role.kubernetes.io/* labels, which is the only
	// place Kubernetes records them.
	Roles   []string `json:"roles"`
	Version string   `json:"version"`
	OS      string   `json:"os"`
	// Pressure lists the conditions that are true right now and should not be —
	// MemoryPressure, DiskPressure, PIDPressure. Empty is the healthy case.
	Pressure    []string  `json:"pressure"`
	Capacity    Resources `json:"capacity"`
	Allocatable Resources `json:"allocatable"`
	// Requested is what the pods scheduled here have reserved, which is what
	// decides whether anything else can be scheduled — independently of whether
	// any of it is being used.
	Requested Resources `json:"requested"`
	// Usage is what is actually being consumed, from metrics-server. Nil when the
	// metrics API is absent or has not yet collected a sample, which is a normal
	// state and not an error.
	Usage *Resources `json:"usage,omitempty"`
	// Filesystem is the node's root disk. Nil unless node stats are switched on:
	// reading them needs a grant on the kubelet that the panel does not hold by
	// default. See charts/admin/templates/api/rbac.yaml.
	Filesystem *Filesystem `json:"filesystem,omitempty"`
	CreatedAt  time.Time   `json:"createdAt"`
}

// VolumeClaim is one PersistentVolumeClaim and the volume behind it.
type VolumeClaim struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	// Status is the claim's phase: Bound, Pending, or Lost. A Pending claim is
	// usually the reason a StatefulSet's pod will not start.
	Status string `json:"status"`
	// Requested is what the claim asked for and Capacity what it actually got.
	// A provisioner may round up, so the two disagreeing is normal rather than a
	// fault — but only Capacity says how much space there really is.
	RequestedBytes int64  `json:"requestedBytes"`
	CapacityBytes  int64  `json:"capacityBytes"`
	StorageClass   string `json:"storageClass"`
	// VolumeName is the PersistentVolume bound to this claim, empty while pending.
	VolumeName  string    `json:"volumeName"`
	AccessModes []string  `json:"accessModes"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Volume is one PersistentVolume.
//
// Reported alongside the claims rather than instead of them, because the two
// answer different questions: the claims say what workloads asked for, and the
// volumes say what exists — including a Released volume still holding data that
// nothing is using and nothing is reclaiming.
type Volume struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	CapacityBytes int64  `json:"capacityBytes"`
	StorageClass  string `json:"storageClass"`
	// Claim is the namespace/name this volume is bound to, empty when unbound.
	Claim string `json:"claim"`
	// ReclaimPolicy decides what happens to the data when the claim goes away,
	// which is the field worth seeing before deleting anything.
	ReclaimPolicy string    `json:"reclaimPolicy"`
	AccessModes   []string  `json:"accessModes"`
	CreatedAt     time.Time `json:"createdAt"`
}

// Storage is the cluster's persistent storage, from both ends.
type Storage struct {
	Claims  []VolumeClaim `json:"claims"`
	Volumes []Volume      `json:"volumes"`
}

// Summary is the dashboard's single round trip.
//
// It is a rollup of what the other endpoints report in detail, computed here so
// the overview page is one call rather than five — and so the arithmetic has one
// implementation with tests rather than one per caller.
type Summary struct {
	Nodes      NodeSummary     `json:"nodes"`
	Pods       PodSummary      `json:"pods"`
	Workloads  WorkloadSummary `json:"workloads"`
	Storage    StorageSummary  `json:"storage"`
	Namespaces int             `json:"namespaces"`
	// MetricsAvailable reports whether metrics-server answered. When false the
	// usage figures are zero because nothing measured them, not because nothing
	// is running — a distinction the dashboard has to draw for the operator.
	MetricsAvailable bool `json:"metricsAvailable"`
}

// NodeSummary rolls up the cluster's capacity and what is claimed against it.
type NodeSummary struct {
	Total int `json:"total"`
	Ready int `json:"ready"`
	// Pressure counts nodes reporting any pressure condition.
	Pressure    int       `json:"pressure"`
	Allocatable Resources `json:"allocatable"`
	Requested   Resources `json:"requested"`
	// Usage is the sum over the nodes that reported one, and is meaningless
	// unless Summary.MetricsAvailable is true — a sum of nothing is zero, and
	// zero is also what a genuinely idle cluster reports. This is a plain value
	// rather than the pointer Node.Usage is because a total has one flag covering
	// it, where a per-node reading has to say which nodes were measured.
	//
	// Even with MetricsAvailable true, a node that metrics-server has not sampled
	// yet contributes nothing, so this can understate a cluster that is still
	// warming up.
	Usage Resources `json:"usage"`
}

// PodSummary counts pods by the states worth acting on.
type PodSummary struct {
	Total    int `json:"total"`
	Running  int `json:"running"`
	Pending  int `json:"pending"`
	Failed   int `json:"failed"`
	Restarts int `json:"restarts"`
}

// WorkloadSummary counts controllers and how many are short of their replicas.
type WorkloadSummary struct {
	Total int `json:"total"`
	// Degraded is the count with fewer ready replicas than desired, which is the
	// number that answers "is anything wrong" on its own.
	Degraded int `json:"degraded"`
}

// StorageSummary rolls up the persistent volume claims.
type StorageSummary struct {
	Claims int `json:"claims"`
	// Unbound is every claim that is not Bound, which is Pending and Lost
	// together. They are counted as one because they mean the same thing to the
	// operator — a workload asked for storage and does not have it — and named
	// for what they have in common rather than for the more common of the two.
	Unbound int `json:"unbound"`
	// CapacityBytes counts only bound claims. An unbound claim has no capacity
	// yet, and counting what it asked for would report space that does not exist.
	CapacityBytes int64 `json:"capacityBytes"`
}

// The workload kinds this reports and, for two of them, acts on.
const (
	KindDeployment  = "Deployment"
	KindStatefulSet = "StatefulSet"
	KindDaemonSet   = "DaemonSet"
)

// Scale is a workload's replica count: what was asked for, and what there is.
type Scale struct {
	// Replicas is the desired count — what the controller was told to run.
	Replicas int32 `json:"replicas"`
	// Current is how many exist right now, which lags Replicas during a rollout.
	Current int32 `json:"current"`
}

// ClusterService is one Kubernetes Service — the thing that gives a workload an
// address.
//
// Not named Service, because this slice already has one: the layer between the
// handler and the repository. Prefixing the DTO rather than renaming that keeps
// every slice's three layers spelled the same way.
type ClusterService struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	// Type is ClusterIP, NodePort, LoadBalancer, or ExternalName.
	Type      string `json:"type"`
	ClusterIP string `json:"clusterIP"`
	// Ports are rendered as "80→8080/TCP", which is the form that answers what
	// the caller connects to and what the container is listening on.
	Ports    []string `json:"ports"`
	Selector []string `json:"selector"`
}

// WorkloadDetail is everything one workload's page shows, in one round trip.
//
// Assembled here rather than left to the caller because the pieces are found by
// following the workload's own selector, and a client doing that would have to
// know how Kubernetes labels relate to each other.
type WorkloadDetail struct {
	Workload Workload `json:"workload"`
	// Scale is absent for a DaemonSet, which has no replica count to set.
	Scale *Scale `json:"scale,omitempty"`
	// Replicas is what the controller reports beyond ready — updated and
	// available, which is how a rollout that has stalled is told from one that is
	// merely in progress.
	Updated   int32 `json:"updated"`
	Available int32 `json:"available"`
	// Conditions are the controller's own, which carry the reason a rollout is
	// stuck when the replica counts only show that it is.
	Conditions []Condition      `json:"conditions"`
	Pods       []Pod            `json:"pods"`
	Services   []ClusterService `json:"services"`
	Claims     []VolumeClaim    `json:"claims"`
	// Events are scoped to this workload and its pods, most recent first.
	Events []Event `json:"events"`
}

// Condition is one controller condition.
type Condition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	// LastTransition is when it last changed, which is what says whether a
	// failing condition is new or has been there all along.
	LastTransition time.Time `json:"lastTransition"`
}

// ScaleRequest is what a caller sends to change a replica count.
type ScaleRequest struct {
	// A pointer so absent can be told from zero. int32 would make `{}` decode to
	// 0, and 0 is a legitimate replica count — so a request that named no count
	// at all would scale the workload down to nothing. Scaling to zero has to be
	// something the caller asked for.
	Replicas *int32 `json:"replicas"`
}
