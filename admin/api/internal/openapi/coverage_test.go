package openapi

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// CI regenerates the spec and fails on a diff, which catches an annotation edited
// without regenerating. It does not catch the opposite and more likely mistake: a
// route added with no annotation at all. That regenerates to byte-identical
// output, produces no diff, and passes — leaving the description quietly claiming
// to describe an API it is missing a piece of.
//
// So these read the routes back out of the source and compare them with the spec.
// They are coarse by design: a route is registered in exactly one way in this
// module — mux.HandleFunc("METHOD /path", …) — and matching that literal is
// cheaper and harder to fool than building the real server, which would need an
// identity provider, a database, and a cluster.

// A route registration, e.g. mux.HandleFunc("GET /api/whoami", h.whoami).
var routePattern = regexp.MustCompile(`HandleFunc\(\s*"([A-Z]+)\s+([^"]+)"`)

func TestEveryRegisteredRouteIsDescribed(t *testing.T) {
	registered := registeredRoutes(t)
	if len(registered) == 0 {
		t.Fatal("found no registered routes; the scan is broken, not the spec")
	}

	described := describedRoutes(t)

	var missing []string
	for route := range registered {
		if _, ok := described[route]; !ok {
			missing = append(missing, route)
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		t.Errorf("these routes are registered but not described:\n  %s\n\n"+
			"annotate the handler, then run `task admin-api:openapi`",
			strings.Join(missing, "\n  "))
	}
}

// The other half of the same claim: a described route that nothing serves sends a
// caller somewhere that answers 404.
func TestEveryDescribedRouteIsServed(t *testing.T) {
	registered := registeredRoutes(t)
	described := describedRoutes(t)

	var phantom []string
	for route := range described {
		if _, ok := registered[route]; !ok {
			phantom = append(phantom, route)
		}
	}
	sort.Strings(phantom)

	if len(phantom) > 0 {
		t.Errorf("these routes are described but nothing serves them:\n  %s\n\n"+
			"remove the annotation, or register the handler",
			strings.Join(phantom, "\n  "))
	}
}

// Endpoints under /api sit behind the bearer-token middleware, so the description
// has to say so. A caller reading the document is entitled to know which requests
// need a credential before it makes one, and an agent has nothing else to go on.
func TestEveryAPIRouteDeclaresItsSecurity(t *testing.T) {
	var document struct {
		Paths map[string]map[string]struct {
			Security []map[string][]string `json:"security"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(Spec(), &document); err != nil {
		t.Fatalf("parse the embedded spec: %v", err)
	}

	for path, operations := range document.Paths {
		if !strings.HasPrefix(path, "/api/") {
			continue
		}
		for method, operation := range operations {
			if len(operation.Security) == 0 {
				t.Errorf("%s %s is under /api but declares no security requirement", strings.ToUpper(method), path)
			}
		}
	}
}

// registeredRoutes scans the module's non-test source for route registrations.
// Test files are excluded deliberately: they stand up throwaway muxes — the fake
// identity provider in internal/auth is one — and those routes are not this API.
func registeredRoutes(t *testing.T) map[string]struct{} {
	t.Helper()

	moduleRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve the module root: %v", err)
	}

	routes := make(map[string]struct{})
	walkErr := filepath.WalkDir(moduleRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "bin" || strings.HasPrefix(entry.Name(), ".") && entry.Name() != "." {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		source, readErr := os.ReadFile(path) //nolint:gosec // paths come from walking this module
		if readErr != nil {
			return readErr
		}
		for _, match := range routePattern.FindAllStringSubmatch(string(source), -1) {
			routes[match[1]+" "+match[2]] = struct{}{}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("scan the module for route registrations: %v", walkErr)
	}
	return routes
}

// describedRoutes reads "METHOD /path" out of the embedded document.
func describedRoutes(t *testing.T) map[string]struct{} {
	t.Helper()

	var document struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(Spec(), &document); err != nil {
		t.Fatalf("parse the embedded spec: %v", err)
	}

	routes := make(map[string]struct{}, len(document.Paths))
	for path, operations := range document.Paths {
		for method := range operations {
			routes[strings.ToUpper(method)+" "+path] = struct{}{}
		}
	}
	return routes
}
