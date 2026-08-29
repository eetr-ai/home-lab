package helm

import (
	"testing"

	releasev1 "helm.sh/helm/v4/pkg/release/v1"

	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/release/common"
)

// Translating one of Helm's releases into this slice's, against Helm's real
// types rather than a fake.
//
// The service tests substitute the repository, so nothing there ever exercised
// this function — and it shipped returning an empty chart name and version for
// every release, which is not a cosmetic gap: the pipeline's completion check is
// "deployed AND the chart version is the one I asked for", and an upgrade finds
// its catalog entry by the chart name it reads back here. Both were broken and
// both looked fine.
//
// The cause is worth a test rather than a comment alone: Helm's MetadataAsMap is
// keyed by Go field name, not by the JSON tag on the same struct, so `version`
// misses and `Version` hits.
func TestReleaseFromReadsTheChartMetadata(t *testing.T) {
	release := &releasev1.Release{
		Name:      "demo",
		Namespace: "apps",
		Version:   3,
		Info: &releasev1.Info{
			Status:      common.StatusDeployed,
			Description: "Upgrade complete",
		},
		Chart: &chartv2.Chart{
			Metadata: &chartv2.Metadata{
				Name:       "podinfo",
				Version:    "6.7.1",
				AppVersion: "6.7.1",
			},
		},
	}

	got, err := releaseFrom(release)
	if err != nil {
		t.Fatalf("releaseFrom: %v", err)
	}

	for _, field := range []struct{ name, got, want string }{
		{"name", got.Name, "demo"},
		{"namespace", got.Namespace, "apps"},
		{"status", got.Status, "deployed"},
		{"chart", got.Chart, "podinfo"},
		{"chartVersion", got.ChartVersion, "6.7.1"},
		{"appVersion", got.AppVersion, "6.7.1"},
		{"description", got.Description, "Upgrade complete"},
	} {
		if field.got != field.want {
			t.Errorf("%s = %q, want %q", field.name, field.got, field.want)
		}
	}
	if got.Revision != 3 {
		t.Errorf("revision = %d, want 3", got.Revision)
	}
}

// A chart with no metadata at all must not take the whole listing down with it.
func TestReleaseFromToleratesAChartWithNoMetadata(t *testing.T) {
	release := &releasev1.Release{
		Name:  "bare",
		Info:  &releasev1.Info{Status: common.StatusFailed},
		Chart: &chartv2.Chart{},
	}

	got, err := releaseFrom(release)
	if err != nil {
		t.Fatalf("releaseFrom: %v", err)
	}
	if got.Name != "bare" || got.Status != "failed" {
		t.Errorf("got %+v, want the release still readable", got)
	}
	if got.Chart != "" || got.ChartVersion != "" {
		t.Errorf("chart = %q/%q, want empty rather than invented", got.Chart, got.ChartVersion)
	}
}
