package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eetr-ai/home-lab/admin/api/internal/nspolicy"
)

// fakeRepo records the namespace it was asked about, so a test can check that an
// invalid one was refused before any request reached the cluster.
type fakeRepo struct {
	asked []string
	// tail records what the log request was narrowed to, so a test can check the
	// cap was applied rather than passed through.
	tail int64
	// namespaces is what ReadNamespace answers with, keyed by name. A name that
	// is not here reads as absent.
	namespaces map[string]Namespace
	// workloads is how many ListWorkloads reports, so a delete test can put
	// something in a namespace without building a workload.
	workloads int
	// created and deleted record the writes that reached the cluster, which is how
	// a test asserts that a refusal happened before anything did.
	created []NamespaceSpec
	deleted []string
	// deletedUID is the precondition the delete carried, so a test can assert the
	// object deleted is the one that was read.
	deletedUID string
	// secrets records the Secret writes that reached the cluster, and whether each
	// was a create or an overwriting update — the difference is what stops a
	// running release having its credential replaced by accident.
	secrets []writtenSecret
	// secretErr is what the next Secret write answers with, so a test can put a
	// conflict in front of the service without a cluster.
	secretErr error
	// live is what ReadSecret and ListSecrets answer with, keyed by name, holding
	// the values as well so a rotation can be checked for leaving the other keys
	// alone. Nothing the service can reach ever sees this map — that is the point
	// of it being here and of SecretSummary having nowhere to put a value.
	live map[string]liveSecret
	// removed records the Secrets a delete reached the cluster with, as the
	// service handed them over — so a test can assert the delete was bound to the
	// object that was checked and not just to its name.
	removed []SecretSummary
	// rotated records each rotation as the fake merged it, so a test can assert
	// the keys not named kept their values.
	rotated []map[string]string
}

// liveSecret is a Secret as the cluster holds it: the projection the repository
// would return, plus the values the repository never hands over.
type liveSecret struct {
	summary SecretSummary
	data    map[string]string
}

// writtenSecret is one Secret write as the fake saw it.
type writtenSecret struct {
	namespace string
	spec      SecretSpec
	overwrote bool
}

func (f *fakeRepo) ReadNamespace(_ context.Context, name string) (Namespace, error) {
	f.asked = append(f.asked, "namespace:"+name)
	namespace, ok := f.namespaces[name]
	if !ok {
		return Namespace{}, ErrNotFound
	}
	return namespace, nil
}

func (f *fakeRepo) CreateNamespace(_ context.Context, spec NamespaceSpec) (Namespace, error) {
	f.created = append(f.created, spec)
	return Namespace{Name: spec.Name, Labels: spec.Labels}, nil
}

func (f *fakeRepo) DeleteNamespace(_ context.Context, name, uid string) error {
	f.deletedUID = uid
	f.deleted = append(f.deleted, name)
	return nil
}

func (f *fakeRepo) ListNamespaces(context.Context) ([]Namespace, error) {
	f.asked = append(f.asked, "namespaces")
	return []Namespace{{Name: "default"}}, nil
}

func (f *fakeRepo) ListWorkloads(_ context.Context, namespace string) ([]Workload, error) {
	f.asked = append(f.asked, "workloads:"+namespace)
	return make([]Workload, f.workloads), nil
}

func (f *fakeRepo) ListPods(_ context.Context, namespace string) ([]Pod, error) {
	f.asked = append(f.asked, "pods:"+namespace)
	return nil, nil
}

func (f *fakeRepo) ListEvents(_ context.Context, namespace string) ([]Event, error) {
	f.asked = append(f.asked, "events:"+namespace)
	return nil, nil
}

func (f *fakeRepo) ListNodes(context.Context) ([]Node, error) {
	f.asked = append(f.asked, "nodes")
	return nil, nil
}

func (f *fakeRepo) ReadStorage(context.Context) (Storage, error) {
	f.asked = append(f.asked, "storage")
	return Storage{}, nil
}

func (f *fakeRepo) ReadSummary(context.Context) (Summary, error) {
	f.asked = append(f.asked, "summary")
	return Summary{}, nil
}

func (f *fakeRepo) ReadWorkload(_ context.Context, kind, namespace, name string) (WorkloadDetail, error) {
	f.asked = append(f.asked, "workload:"+kind+"/"+namespace+"/"+name)
	return WorkloadDetail{}, nil
}

func (f *fakeRepo) PodLogs(_ context.Context, namespace, pod string, options LogOptions) (io.ReadCloser, error) {
	f.asked = append(f.asked, "logs:"+namespace+"/"+pod)
	f.tail = options.Tail
	return io.NopCloser(strings.NewReader("")), nil
}

func (f *fakeRepo) RestartWorkload(_ context.Context, kind, namespace, name string, _ time.Time) error {
	f.asked = append(f.asked, "restart:"+kind+"/"+namespace+"/"+name)
	return nil
}

func (f *fakeRepo) ScaleWorkload(_ context.Context, kind, namespace, name string, replicas int32) error {
	f.asked = append(f.asked, fmt.Sprintf("scale:%s/%s/%s=%d", kind, namespace, name, replicas))
	return nil
}

func (f *fakeRepo) CreateSecret(_ context.Context, namespace string, spec SecretSpec) error {
	if f.secretErr != nil {
		return f.secretErr
	}
	f.secrets = append(f.secrets, writtenSecret{namespace: namespace, spec: spec})
	return nil
}

func (f *fakeRepo) UpdateSecret(_ context.Context, namespace string, spec SecretSpec) error {
	if f.secretErr != nil {
		return f.secretErr
	}
	f.secrets = append(f.secrets, writtenSecret{namespace: namespace, spec: spec, overwrote: true})
	return nil
}

func (f *fakeRepo) ListSecrets(_ context.Context, namespace string) ([]SecretSummary, error) {
	f.asked = append(f.asked, "secrets:"+namespace)
	if f.secretErr != nil {
		return nil, f.secretErr
	}
	secrets := make([]SecretSummary, 0, len(f.live))
	for _, secret := range f.live {
		secrets = append(secrets, secret.summary)
	}
	sort.Slice(secrets, func(i, j int) bool { return secrets[i].Name < secrets[j].Name })
	return secrets, nil
}

func (f *fakeRepo) ReadSecret(_ context.Context, namespace, name string) (SecretSummary, error) {
	f.asked = append(f.asked, "secret:"+namespace+"/"+name)
	secret, ok := f.live[name]
	if !ok {
		return SecretSummary{}, ErrNotFound
	}
	return secret.summary, nil
}

func (f *fakeRepo) DeleteSecret(_ context.Context, _ string, target SecretSummary) error {
	if f.secretErr != nil {
		return f.secretErr
	}
	f.removed = append(f.removed, target)
	return nil
}

// The merge is the repository's job in the real one, so the fake does it too —
// otherwise a test asserting the untouched keys survived would be asserting
// against nothing.
func (f *fakeRepo) RotateSecretKeys(
	_ context.Context, _, name string, values map[string]string,
) error {
	if f.secretErr != nil {
		return f.secretErr
	}
	merged := map[string]string{}
	for key, value := range f.live[name].data {
		merged[key] = value
	}
	for key, value := range values {
		merged[key] = value
	}
	f.rotated = append(f.rotated, merged)
	return nil
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
			service := newTestService(repo)

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

// A pod name is a DNS subdomain, not a label. Validating it as a label would
// refuse real pods — a Deployment named long enough that its pods run past 63
// characters is perfectly ordinary — with a message blaming the caller for a rule
// that does not apply to them.
func TestPodNamesAreValidatedAsSubdomains(t *testing.T) {
	tests := []struct {
		name    string
		pod     string
		wantErr bool
	}{
		{name: "an ordinary deployment pod", pod: "admin-api-7d9f8b6c5d-x4k2p"},
		{name: "a statefulset pod", pod: "mongo-0"},
		{name: "a dotted name, which a subdomain permits", pod: "some.pod.name"},
		{name: "long, but under the subdomain limit", pod: strings.Repeat("a", 200)},
		{name: "past the subdomain limit", pod: strings.Repeat("a", 254), wantErr: true},
		{name: "uppercase", pod: "Admin-API", wantErr: true},
		{name: "a path traversal attempt", pod: "../../secrets", wantErr: true},
		{name: "empty", pod: "", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepo{}
			_, err := newTestService(repo).PodLogs(t.Context(), "default", test.pod, LogOptions{})

			if test.wantErr {
				if !errors.Is(err, ErrInvalidName) {
					t.Fatalf("PodLogs(%q) error = %v, want %v", test.pod, err, ErrInvalidName)
				}
				if len(repo.asked) != 0 {
					t.Errorf("%q was refused but still reached the cluster: %v", test.pod, repo.asked)
				}
				return
			}
			if err != nil {
				t.Fatalf("PodLogs(%q) error = %v", test.pod, err)
			}
		})
	}
}

// An unbounded tail holds the connection for as long as it takes to send a pod's
// whole history, which is not a request anybody makes on purpose.
func TestLogTailIsCapped(t *testing.T) {
	tests := []struct {
		name string
		ask  int64
		want int64
	}{
		{name: "unset falls back to the default", ask: 0, want: defaultLogTail},
		{name: "negative falls back to the default", ask: -1, want: defaultLogTail},
		{name: "a reasonable ask is honoured", ask: 500, want: 500},
		{name: "at the cap is honoured", ask: maxLogTail, want: maxLogTail},
		{name: "past the cap falls back to the default", ask: maxLogTail + 1, want: defaultLogTail},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepo{}
			if _, err := newTestService(repo).PodLogs(
				t.Context(), "default", "pod", LogOptions{Tail: test.ask}); err != nil {
				t.Fatalf("PodLogs error = %v", err)
			}
			if repo.tail != test.want {
				t.Errorf("tail = %d; want %d", repo.tail, test.want)
			}
		})
	}
}

// A DaemonSet has no replica count to set — its size is however many nodes it
// matches — so the request must be refused rather than sent and rejected.
func TestUnsupportedKindsAreRefused(t *testing.T) {
	repo := &fakeRepo{}
	service := newTestService(repo)

	if err := service.ScaleWorkload(t.Context(), KindDaemonSet, "default", "node-exporter", 3); !errors.Is(err, ErrUnsupportedKind) {
		t.Errorf("ScaleWorkload(DaemonSet) error = %v, want %v", err, ErrUnsupportedKind)
	}
	if err := service.RestartWorkload(t.Context(), KindDaemonSet, "default", "node-exporter"); !errors.Is(err, ErrUnsupportedKind) {
		t.Errorf("RestartWorkload(DaemonSet) error = %v, want %v", err, ErrUnsupportedKind)
	}
	if err := service.ScaleWorkload(t.Context(), "Pod", "default", "thing", 3); !errors.Is(err, ErrInvalidName) {
		t.Errorf("ScaleWorkload(Pod) error = %v, want %v", err, ErrInvalidName)
	}
	if len(repo.asked) != 0 {
		t.Errorf("a refused write still reached the cluster: %v", repo.asked)
	}
}

// A mistyped replica count must not reach the scheduler as a request for
// thousands of pods.
func TestReplicaCountIsBounded(t *testing.T) {
	tests := []struct {
		name     string
		replicas int32
		wantErr  bool
	}{
		{name: "scaling to zero is a real thing to want", replicas: 0},
		{name: "an ordinary count", replicas: 3},
		{name: "at the cap", replicas: maxReplicas},
		{name: "past the cap", replicas: maxReplicas + 1, wantErr: true},
		{name: "negative", replicas: -1, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepo{}
			err := newTestService(repo).ScaleWorkload(t.Context(), KindDeployment, "default", "api", test.replicas)

			if test.wantErr {
				if !errors.Is(err, ErrInvalidName) {
					t.Fatalf("error = %v, want %v", err, ErrInvalidName)
				}
				if len(repo.asked) != 0 {
					t.Errorf("a refused scale still reached the cluster: %v", repo.asked)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			want := fmt.Sprintf("scale:Deployment/default/api=%d", test.replicas)
			if len(repo.asked) != 1 || repo.asked[0] != want {
				t.Errorf("calls = %v; want [%s]", repo.asked, want)
			}
		})
	}
}

// newTestService builds a service with the policy this lab actually runs: the
// panel in "admin" and platform-system protected by configuration.
//
// No enrolment: these tests are about namespaces and workloads, and a nil
// enrolment is the shape a lab that does not deploy from the panel has. The
// enrolment paths have their own tests.
func newTestService(repo repository) *Service {
	service, err := NewService(repo, nspolicy.New(nspolicy.Config{
		Own:       "admin",
		Protected: []string{"platform-system"},
	}), "", nil)
	if err != nil {
		panic(err)
	}
	return service
}
