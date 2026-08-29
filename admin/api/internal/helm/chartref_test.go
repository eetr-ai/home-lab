package helm

import (
	"errors"
	"testing"
)

// ParseChartRef is the only thing between a request and an arbitrary fetch now
// that there is no catalog, so what it refuses matters more than what it accepts.
func TestParseChartRef(t *testing.T) {
	cases := []struct {
		name       string
		reference  string
		wantChart  string
		wantURL    string
		wantOCI    bool
		wantRefuse bool
	}{
		{
			name:      "an OCI reference splits at the last segment",
			reference: "oci://ghcr.io/stefanprodan/charts/podinfo",
			wantChart: "podinfo",
			wantURL:   "oci://ghcr.io/stefanprodan/charts",
			wantOCI:   true,
		},
		{
			name:      "an OCI reference directly under the registry",
			reference: "oci://localhost:5001/podinfo",
			wantChart: "podinfo",
			wantURL:   "oci://localhost:5001",
			wantOCI:   true,
		},
		{
			name:      "an HTTPS reference splits the same way",
			reference: "https://stefanprodan.github.io/podinfo/podinfo",
			wantChart: "podinfo",
			wantURL:   "https://stefanprodan.github.io/podinfo",
		},
		{
			name:      "surrounding whitespace is forgiven",
			reference: "  oci://ghcr.io/org/chart  ",
			wantChart: "chart",
			wantURL:   "oci://ghcr.io/org",
			wantOCI:   true,
		},
		{
			name:      "a trailing slash is not a chart name",
			reference: "oci://ghcr.io/org/chart/",
			wantChart: "chart",
			wantURL:   "oci://ghcr.io/org",
			wantOCI:   true,
		},

		{name: "empty", reference: "", wantRefuse: true},
		{name: "no scheme", reference: "ghcr.io/org/chart", wantRefuse: true},
		{name: "plain http", reference: "http://example.com/repo/chart", wantRefuse: true},
		{name: "a local path", reference: "file:///etc/passwd", wantRefuse: true},
		{name: "no host", reference: "oci:///chart", wantRefuse: true},
		{name: "no chart", reference: "oci://ghcr.io", wantRefuse: true},
		{name: "a query", reference: "https://example.com/repo/chart?token=x", wantRefuse: true},
		{name: "a fragment", reference: "https://example.com/repo/chart#v1", wantRefuse: true},
		// The one that matters most: credentials here would be written to the
		// deployment record and repeated in every log line naming the chart.
		//nolint:gosec // deliberately credential-bearing; this asserts it is refused
		{
			name:       "embedded credentials",
			reference:  "oci://user:secret@ghcr.io/org/chart",
			wantRefuse: true,
		},
		{name: "a chart name that is not one", reference: "oci://ghcr.io/org/ch art", wantRefuse: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			source, err := ParseChartRef(testCase.reference)

			if testCase.wantRefuse {
				if !errors.Is(err, ErrInvalidChartRef) {
					t.Fatalf("parsing %q: want ErrInvalidChartRef, got %v", testCase.reference, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("parsing %q: %v", testCase.reference, err)
			}
			if source.Chart != testCase.wantChart {
				t.Errorf("chart: want %q, got %q", testCase.wantChart, source.Chart)
			}
			if source.URL != testCase.wantURL {
				t.Errorf("url: want %q, got %q", testCase.wantURL, source.URL)
			}
			if source.OCI != testCase.wantOCI {
				t.Errorf("oci: want %v, got %v", testCase.wantOCI, source.OCI)
			}
		})
	}
}

// A parsed reference rebuilds into what was typed, because the panel shows it
// back and a deployment is identified by it.
func TestChartSourceRebuildsItsReference(t *testing.T) {
	for _, reference := range []string{
		"oci://ghcr.io/stefanprodan/charts/podinfo",
		"oci://localhost:5001/podinfo",
		"https://stefanprodan.github.io/podinfo/podinfo",
	} {
		source, err := ParseChartRef(reference)
		if err != nil {
			t.Fatalf("parsing %q: %v", reference, err)
		}
		if source.Ref() != reference {
			t.Errorf("want %q, got %q", reference, source.Ref())
		}
	}
}

// A version has to be one exact version. A range would mean the lab installs
// whatever satisfies it on the day it runs, which is the opposite of pinning.
func TestValidateVersion(t *testing.T) {
	valid := []string{"1.2.3", "v1.2.3", "0.0.1", "1.2.3-rc.1", "1.2.3+build.5"}
	for _, version := range valid {
		if err := validateVersion(version); err != nil {
			t.Errorf("%q should be accepted: %v", version, err)
		}
	}

	invalid := []string{"", "latest", "^1.2.0", "~1.2", ">=1.0.0", "1.2", "main", "1.2.3 "}
	for _, version := range invalid {
		if err := validateVersion(version); !errors.Is(err, ErrInvalidName) {
			t.Errorf("%q should be refused, got %v", version, err)
		}
	}
}
