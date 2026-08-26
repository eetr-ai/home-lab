package kube

import "testing"

func TestRollUpNodes(t *testing.T) {
	measured := Resources{CPUMillis: 300, MemoryBytes: 900}
	nodes := []Node{
		{
			Ready:       true,
			Allocatable: Resources{CPUMillis: 4000, MemoryBytes: 8000},
			Requested:   Resources{CPUMillis: 1000, MemoryBytes: 2000},
			Usage:       &measured,
		},
		{
			// Down and under pressure, and its capacity still counts: the cluster
			// has it, which is why "2 of 3 ready" is worth saying beside the total.
			Ready:       false,
			Pressure:    []string{"DiskPressure"},
			Allocatable: Resources{CPUMillis: 2000, MemoryBytes: 4000},
			Requested:   Resources{CPUMillis: 500, MemoryBytes: 1000},
		},
	}

	got := rollUpNodes(nodes)
	if got.Total != 2 || got.Ready != 1 || got.Pressure != 1 {
		t.Errorf("counts = total %d, ready %d, pressure %d; want 2, 1, 1",
			got.Total, got.Ready, got.Pressure)
	}
	if got.Allocatable.CPUMillis != 6000 || got.Requested.CPUMillis != 1500 {
		t.Errorf("cpu = allocatable %d, requested %d; want 6000, 1500",
			got.Allocatable.CPUMillis, got.Requested.CPUMillis)
	}
	// Only the node that reported a reading contributes. The other adds nothing
	// rather than adding zero, which is the same number here and not the same claim.
	if got.Usage.CPUMillis != 300 {
		t.Errorf("usage CPUMillis = %d; want 300", got.Usage.CPUMillis)
	}
}

func TestRollUpPods(t *testing.T) {
	pods := []Pod{
		{Phase: "Running", Restarts: 2},
		{Phase: "Running"},
		{Phase: "Pending"},
		{Phase: "Failed", Restarts: 5},
		{Phase: "Succeeded"},
	}

	got := rollUpPods(pods)
	if got.Total != 5 || got.Running != 2 || got.Pending != 1 || got.Failed != 1 {
		t.Errorf("counts = %+v; want total 5, running 2, pending 1, failed 1", got)
	}
	if got.Restarts != 7 {
		t.Errorf("Restarts = %d; want 7", got.Restarts)
	}
}

// A workload scaled deliberately to zero wants none and has none. Reporting it as
// degraded would put a permanent fault on the dashboard.
func TestRollUpWorkloads(t *testing.T) {
	workloads := []Workload{
		{Desired: 3, Ready: 3},
		{Desired: 3, Ready: 1},
		{Desired: 0, Ready: 0},
	}

	got := rollUpWorkloads(workloads)
	if got.Total != 3 || got.Degraded != 1 {
		t.Errorf("rollUpWorkloads() = %+v; want total 3, degraded 1", got)
	}
}

// An unbound claim has no capacity yet. Counting what it asked for would report
// space that does not exist — and a Lost claim is unbound too, not pending.
func TestRollUpStorage(t *testing.T) {
	claims := []VolumeClaim{
		{Status: "Bound", RequestedBytes: 1000, CapacityBytes: 1024},
		{Status: "Bound", RequestedBytes: 2000, CapacityBytes: 2048},
		{Status: "Pending", RequestedBytes: 9999},
		{Status: "Lost", RequestedBytes: 5000},
	}

	got := rollUpStorage(claims)
	if got.Claims != 4 || got.Unbound != 2 {
		t.Errorf("counts = %+v; want claims 4, unbound 2", got)
	}
	if got.CapacityBytes != 3072 {
		t.Errorf("CapacityBytes = %d; want 3072", got.CapacityBytes)
	}
}
