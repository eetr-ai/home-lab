package kube

import "time"

// Namespace is a namespace in the cluster.
type Namespace struct {
	Name   string    `json:"name"`
	Status string    `json:"status"`
	Age    time.Time `json:"createdAt"`
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
	Ready     string    `json:"ready"`
	Restarts  int32     `json:"restarts"`
	Node      string    `json:"node"`
	CreatedAt time.Time `json:"createdAt"`
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
