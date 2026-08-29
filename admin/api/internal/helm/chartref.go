package helm

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ChartSource is a chart to fetch, split the way Helm wants it.
//
// URL is the repository — an OCI registry path or an HTTP chart repository — and
// Chart is the name within it. Helm needs the two separately: for an OCI registry
// it wants them joined back into one reference, and for an HTTP repository it
// wants the URL in ChartPathOptions.RepoURL and the name on its own.
type ChartSource struct {
	Chart string `json:"chart"`
	URL   string `json:"url"`
	OCI   bool   `json:"oci"`
}

// Ref returns the reference an operator typed, rebuilt from its parts.
func (s ChartSource) Ref() string {
	return strings.TrimSuffix(s.URL, "/") + "/" + s.Chart
}

// chartNamePattern is what a chart may be called.
//
// Deliberately narrow. This ends up in a URL path and in a release's stored
// metadata, and a chart name is a Helm identifier rather than free text.
var chartNamePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$`)

// ParseChartRef turns one reference into the source Helm needs.
//
// This is the only thing standing between a request and an arbitrary fetch, now
// that there is no catalog: an operator names a chart by where it lives, and this
// decides whether that is somewhere this API is willing to look. It bounds the
// scheme and refuses embedded credentials, and it does not bound the host — that
// is a deliberate, temporary gap, and the place a repository allowlist goes when
// this lab grows one.
//
// Both accepted forms name the chart in the last path segment:
//
//	oci://ghcr.io/stefanprodan/charts/podinfo
//	https://stefanprodan.github.io/podinfo/podinfo
//
// which is the same shape in both cases and the one an operator would guess. For
// the HTTP form everything before the last segment is the repository whose
// index.yaml is fetched, which is what `helm install --repo` takes.
func ParseChartRef(raw string) (ChartSource, error) {
	reference := strings.TrimSpace(raw)
	if reference == "" {
		return ChartSource{}, fmt.Errorf("%w: a chart reference is required", ErrInvalidChartRef)
	}

	parsed, err := url.Parse(reference)
	if err != nil {
		return ChartSource{}, fmt.Errorf("%w: %q is not a URL", ErrInvalidChartRef, reference)
	}

	switch parsed.Scheme {
	case "oci", "https":
	case "":
		return ChartSource{}, fmt.Errorf(
			"%w: %q has no scheme — a chart reference starts with oci:// or https://",
			ErrInvalidChartRef, reference)
	default:
		return ChartSource{}, fmt.Errorf(
			"%w: %q uses %s, and only oci:// and https:// are fetched",
			ErrInvalidChartRef, reference, parsed.Scheme)
	}

	// Credentials in the reference would be stored with the deployment record and
	// repeated in every log line that names the chart. Registry credentials belong
	// in a Secret the pod reads, never in something an operator types into a form.
	if parsed.User != nil {
		return ChartSource{}, fmt.Errorf(
			"%w: a chart reference may not carry credentials — configure them as a "+
				"registry secret instead", ErrInvalidChartRef)
	}
	if parsed.Host == "" {
		return ChartSource{}, fmt.Errorf("%w: %q names no host", ErrInvalidChartRef, reference)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return ChartSource{}, fmt.Errorf(
			"%w: a chart reference carries no query or fragment", ErrInvalidChartRef)
	}

	repository, chart, err := splitChart(parsed.Path, reference)
	if err != nil {
		return ChartSource{}, err
	}

	return ChartSource{
		Chart: chart,
		URL:   parsed.Scheme + "://" + parsed.Host + repository,
		OCI:   parsed.Scheme == "oci",
	}, nil
}

// splitChart peels the chart name off the end of the path.
func splitChart(path, reference string) (repository, chart string, err error) {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "", "", fmt.Errorf(
			"%w: %q names a host but no chart — the last path segment is the chart name",
			ErrInvalidChartRef, reference)
	}

	segments := strings.Split(trimmed, "/")
	chart = segments[len(segments)-1]
	if !chartNamePattern.MatchString(chart) {
		return "", "", fmt.Errorf(
			"%w: %q is not a chart name", ErrInvalidChartRef, chart)
	}

	repository = strings.Join(segments[:len(segments)-1], "/")
	if repository != "" {
		repository = "/" + repository
	}
	return repository, chart, nil
}

// ChartVersion is one version a repository offers.
type ChartVersion struct {
	Version string `json:"version"`
	// AppVersion is what the chart says it packages, which is usually what an
	// operator actually recognises — a chart version of 6.9.1 means nothing next
	// to an app version of 1.29.0.
	AppVersion string `json:"appVersion,omitempty"`
}
