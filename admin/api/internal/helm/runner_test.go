package helm

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

// stampRecorder captures the context the stamp was written with.
type stampRecorder struct {
	called bool
	live   bool
}

func (s *stampRecorder) MarkRolledOut(ctx context.Context, _ string, _, _ int) error {
	s.called = true
	s.live = ctx.Err() == nil
	return nil
}

// The stamp must survive an operation that used all of its time.
//
// The operation's context is bounded by the Helm timeout. A deploy that
// legitimately took that long would otherwise arrive at the stamp with a context
// already cancelled, so the release would be up and the record would say it never
// rolled out — drift reported for a deploy that worked, on exactly the slow
// deploys most worth trusting the record about.
func TestStampRolloutOutlivesTheOperationDeadline(t *testing.T) {
	expired, cancel := context.WithCancel(t.Context())
	cancel() // the operation ran out of time

	store := &stampRecorder{}
	stampRollout(expired, store, Deployment{ID: "d1"}, DeploymentVersion{Version: 3},
		Release{Revision: 2}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if !store.called {
		t.Fatal("the stamp was never attempted")
	}
	if !store.live {
		t.Error("the stamp ran on a cancelled context, so a slow deploy records as drift")
	}
}
