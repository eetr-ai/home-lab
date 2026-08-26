package kube

import (
	"context"
	"errors"
	"testing"
)

// fakeRepo records the namespace it was asked about, so a test can check that an
// invalid one was refused before any request reached the cluster.
type fakeRepo struct{ asked []string }

func (f *fakeRepo) ListNamespaces(context.Context) ([]Namespace, error) {
	f.asked = append(f.asked, "namespaces")
	return []Namespace{{Name: "default"}}, nil
}

func (f *fakeRepo) ListWorkloads(_ context.Context, namespace string) ([]Workload, error) {
	f.asked = append(f.asked, "workloads:"+namespace)
	return nil, nil
}

func (f *fakeRepo) ListPods(_ context.Context, namespace string) ([]Pod, error) {
	f.asked = append(f.asked, "pods:"+namespace)
	return nil, nil
}

func (f *fakeRepo) ListEvents(_ context.Context, namespace string) ([]Event, error) {
	f.asked = append(f.asked, "events:"+namespace)
	return nil, nil
}

// A malformed namespace must be refused here rather than sent to the API server,
// whose reply is a message about DNS label formats that does not say which
// parameter was wrong.
func TestNamespaceIsValidatedBeforeTheClusterIsAsked(t *testing.T) {
	calls := map[string]func(*Service, string) error{
		"workloads": func(s *Service, ns string) error { _, err := s.ListWorkloads(t.Context(), ns); return err },
		"pods":      func(s *Service, ns string) error { _, err := s.ListPods(t.Context(), ns); return err },
		"events":    func(s *Service, ns string) error { _, err := s.ListEvents(t.Context(), ns); return err },
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{}
			service := NewService(repo)

			if err := call(service, "Not A Namespace"); !errors.Is(err, ErrInvalidName) {
				t.Fatalf("%s error = %v, want %v", name, err, ErrInvalidName)
			}
			if len(repo.asked) != 0 {
				t.Errorf("%s was refused but still asked the cluster: %v", name, repo.asked)
			}

			if err := call(service, "platform-system"); err != nil {
				t.Fatalf("%s with a valid namespace error = %v", name, err)
			}
			if len(repo.asked) != 1 {
				t.Errorf("%s calls = %v, want exactly one", name, repo.asked)
			}
		})
	}
}
