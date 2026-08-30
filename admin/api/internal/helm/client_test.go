package helm

import (
	"testing"

	"helm.sh/helm/v4/pkg/kube"
)

// TestManagedFieldsManagerIsPinned guards the reason an upgrade from the panel
// used to be refused field by field.
//
// Server-side apply arbitrates by the name of the manager that last wrote a
// field, and Helm derives that name from filepath.Base(os.Args[0]) unless this
// variable is set. Unset, the panel applies as "admin-api" while a release
// installed from a terminal is owned by "helm", and Kubernetes reads one writer
// as two:
//
//	conflicts with "helm": .metadata.labels.helm.sh/chart
//
// The failure is silent in every other way — it needs a release that some other
// Helm client touched first, so no unit test of an upgrade would reach it.
// Deleting the init() this asserts leaves the variable empty and this test red.
func TestManagedFieldsManagerIsPinned(t *testing.T) {
	if kube.ManagedFieldsManager != "helm" {
		t.Fatalf(
			"field manager is %q, want \"helm\": the panel and the helm CLI must "+
				"apply as one writer, or server-side apply refuses every field the "+
				"other owns",
			kube.ManagedFieldsManager)
	}
}
