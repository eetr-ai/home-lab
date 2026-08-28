package kube

import (
	"errors"
	"strings"
	"testing"

	"github.com/eetr-ai/home-lab/admin/api/internal/nspolicy"
)

func TestCreateNamespace(t *testing.T) {
	tests := []struct {
		name    string
		spec    NamespaceSpec
		wantErr error
	}{
		{
			name: "an ordinary namespace",
			spec: NamespaceSpec{Name: "apps"},
		},
		{
			name: "a namespace with labels of its own",
			spec: NamespaceSpec{
				Name:   "apps",
				Labels: map[string]string{"home-lab.example/gateway-access": "true"},
			},
		},
		{
			name:    "a name Kubernetes would not accept",
			spec:    NamespaceSpec{Name: "Apps_1"},
			wantErr: ErrInvalidName,
		},
		{
			name:    "a name longer than a DNS label",
			spec:    NamespaceSpec{Name: strings.Repeat("x", maxNamespaceLength+1)},
			wantErr: ErrInvalidName,
		},
		{
			// Otherwise "create a namespace" is "grant myself privileged".
			name: "a label under kubernetes.io",
			spec: NamespaceSpec{
				Name:   "apps",
				Labels: map[string]string{"pod-security.kubernetes.io/enforce": "privileged"},
			},
			wantErr: ErrInvalidName,
		},
		{
			name: "a label under k8s.io",
			spec: NamespaceSpec{
				Name:   "apps",
				Labels: map[string]string{"something.k8s.io/enforce": "privileged"},
			},
			wantErr: ErrInvalidName,
		},
		{
			// The panel decides protection; a request does not get to claim it.
			name: "the protected label",
			spec: NamespaceSpec{
				Name:   "apps",
				Labels: map[string]string{nspolicy.LabelProtected: "true"},
			},
			wantErr: ErrInvalidName,
		},
		{
			name:    "a namespace the policy protects",
			spec:    NamespaceSpec{Name: "kube-system"},
			wantErr: ErrProtected,
		},
		{
			name:    "the panel's own namespace",
			spec:    NamespaceSpec{Name: "admin"},
			wantErr: ErrProtected,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepo{}
			_, err := newTestService(repo).CreateNamespace(t.Context(), test.spec)

			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr != nil && len(repo.created) != 0 {
				// The refusal has to happen before the cluster hears about it, or
				// the rule is a message rather than a rule.
				t.Fatalf("a refused request still created %v", repo.created)
			}
			if test.wantErr != nil {
				return
			}

			created := repo.created[0]
			for key, want := range map[string]string{
				labelManagedBy:        managedByValue,
				labelPodSecurity:      defaultPodSecurity,
				nspolicy.LabelManaged: "true",
			} {
				if created.Labels[key] != want {
					t.Errorf("label %s = %q, want %q", key, created.Labels[key], want)
				}
			}
			for key, want := range test.spec.Labels {
				if created.Labels[key] != want {
					t.Errorf("the caller's label %s was dropped", key)
				}
			}
		})
	}
}

// A request cannot opt out of Pod Security admission by naming the key itself:
// the panel's labels are applied over the caller's, not under them.
func TestCreateNamespaceOverridesTheCallersPodSecurity(t *testing.T) {
	repo := &fakeRepo{}
	_, err := NewService(repo, nspolicy.New(nspolicy.Config{Own: "admin"}), "restricted").
		CreateNamespace(t.Context(), NamespaceSpec{
			Name:   "apps",
			Labels: map[string]string{"home-lab.example/gateway-access": "true"},
		})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := repo.created[0].Labels[labelPodSecurity]; got != "restricted" {
		t.Errorf("%s = %q, want the configured level", labelPodSecurity, got)
	}
}

func TestDeleteNamespace(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		labels    map[string]string
		workloads int
		force     bool
		wantErr   error
	}{
		{
			name:      "an empty, unprotected namespace",
			namespace: "apps",
		},
		{
			name:      "a namespace protected by configuration",
			namespace: "platform-system",
			wantErr:   ErrProtected,
		},
		{
			name:      "a Kubernetes system namespace",
			namespace: "kube-system",
			wantErr:   ErrProtected,
		},
		{
			name:      "the panel's own namespace",
			namespace: "admin",
			wantErr:   ErrProtected,
		},
		{
			// The label is read off the live object, which is the half of the
			// policy that configuration cannot see.
			name:      "a namespace protected only by its label",
			namespace: "apps",
			labels:    map[string]string{nspolicy.LabelProtected: "true"},
			wantErr:   ErrProtected,
		},
		{
			name:      "a namespace that still runs something",
			namespace: "apps",
			workloads: 4,
			wantErr:   ErrNotEmpty,
		},
		{
			name:      "a namespace that still runs something, forced",
			namespace: "apps",
			workloads: 4,
			force:     true,
		},
		{
			// Force is not a way past protection. It answers "I know it is not
			// empty", and nothing else.
			name:      "a protected namespace, forced",
			namespace: "platform-system",
			force:     true,
			wantErr:   ErrProtected,
		},
		{
			name:      "a name Kubernetes would not accept",
			namespace: "Apps_1",
			wantErr:   ErrInvalidName,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepo{
				workloads: test.workloads,
				namespaces: map[string]Namespace{
					test.namespace: {Name: test.namespace, Labels: test.labels},
				},
			}

			err := newTestService(repo).DeleteNamespace(t.Context(), test.namespace, test.force)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}

			deleted := len(repo.deleted)
			if test.wantErr != nil && deleted != 0 {
				t.Fatalf("a refused delete still removed %v", repo.deleted)
			}
			if test.wantErr == nil && deleted != 1 {
				t.Fatalf("delete reached the cluster %d times, want 1", deleted)
			}
		})
	}
}

// A protected namespace is still listed and still readable. Protection is about
// writing; hiding it would take away the reading this panel exists for.
func TestListNamespacesReportsProtectionWithoutHidingAnything(t *testing.T) {
	repo := &fakeRepo{}
	namespaces, err := newTestService(repo).ListNamespaces(t.Context())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(namespaces) != 1 || namespaces[0].Name != "default" {
		t.Fatalf("namespaces = %v, want the one the repository reported", namespaces)
	}
	if !namespaces[0].Protected {
		t.Error("default is a system namespace and was not reported as protected")
	}
	if namespaces[0].ProtectedReason == "" {
		t.Error("a protected namespace was listed with no reason; the panel renders it")
	}
}
