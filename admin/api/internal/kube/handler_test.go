package kube

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eetr-ai/home-lab/admin/api/internal/auth"
)

// scaleRecorder is a repository that records only what it was asked to scale, so
// a handler test can assert a refused request never reached the cluster.
type scaleRecorder struct {
	fakeRepo
	scaled []int32
}

func (s *scaleRecorder) ScaleWorkload(_ context.Context, _, _, _ string, replicas int32) error {
	s.scaled = append(s.scaled, replicas)
	return nil
}

// A body naming no replica count must be refused, not read as zero.
//
// Replicas is a pointer for exactly this: int32 would make `{}` decode to 0, and
// 0 is a legitimate count — so a request that asked for nothing would take the
// workload down. This is the test that keeps the pointer there.
func TestScaleRequiresAnExplicitCount(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantScaled []int32
	}{
		{
			name: "an explicit count is honoured",
			body: `{"replicas": 3}`, wantStatus: http.StatusNoContent, wantScaled: []int32{3},
		},
		{
			// Scaling to zero is a real thing to want, and must still work when it
			// is what the caller actually wrote.
			name: "an explicit zero is honoured",
			body: `{"replicas": 0}`, wantStatus: http.StatusNoContent, wantScaled: []int32{0},
		},
		{
			name: "an empty object is refused",
			body: `{}`, wantStatus: http.StatusBadRequest,
		},
		{
			name: "an explicit null is refused",
			body: `{"replicas": null}`, wantStatus: http.StatusBadRequest,
		},
		{
			name: "a malformed body is refused",
			body: `not json`, wantStatus: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &scaleRecorder{}
			mux := http.NewServeMux()
			NewHandler(NewService(repo), auth.NewGuard(false)).Register(mux)

			// The route is scoped, so it needs a caller the way Middleware would
			// have left one. Scopeless, which is what the panel presents today.
			ctx := auth.WithSubject(t.Context(), auth.Subject{ID: "operator"})
			request := httptest.NewRequestWithContext(ctx, http.MethodPut,
				"/api/kubernetes/namespaces/default/workloads/Deployment/api/scale",
				strings.NewReader(test.body))
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				body, _ := io.ReadAll(recorder.Body)
				t.Fatalf("status = %d, want %d (%s)", recorder.Code, test.wantStatus, body)
			}
			if len(repo.scaled) != len(test.wantScaled) {
				t.Fatalf("scaled = %v, want %v", repo.scaled, test.wantScaled)
			}
			for i := range test.wantScaled {
				if repo.scaled[i] != test.wantScaled[i] {
					t.Errorf("scaled[%d] = %d, want %d", i, repo.scaled[i], test.wantScaled[i])
				}
			}
		})
	}
}

// The log route clears the server's write deadline before streaming. If something
// starts wrapping the ResponseWriter so that cannot be done, the handler must
// refuse rather than serve a stream that will be cut at thirty seconds with no
// explanation.
func TestLogHandlerRefusesWhenTheDeadlineCannotBeCleared(t *testing.T) {
	recorder := httptest.NewRecorder()
	// httptest.ResponseRecorder implements neither SetWriteDeadline nor Unwrap,
	// which is exactly the shape being guarded against.
	if !clearWriteDeadline(recorder) {
		if recorder.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
		}
		return
	}
	t.Error("clearWriteDeadline reported success on a writer that cannot support it")
}
