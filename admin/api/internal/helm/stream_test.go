package helm

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// streamingJobs feeds a scripted sequence of job states and a scripted log.
type streamingJobs struct {
	fakeJobs
	states []Job
	log    string
	// reads counts calls, so a poll a moment later sees progress the way it
	// would against a real cluster. A fake that answered with the first state
	// forever would leave the log follower waiting for a pod that, in the
	// fiction, has already been scheduled.
	reads int
}

func (s *streamingJobs) ReadJob(_ context.Context, name string) (Job, error) {
	if len(s.states) == 0 {
		return Job{}, ErrNotFound
	}
	found := s.states[min(s.reads, len(s.states)-1)]
	s.reads++
	found.Name = name
	return found, nil
}

func (s *streamingJobs) WatchJob(_ context.Context, _ string) (<-chan Job, error) {
	updates := make(chan Job, len(s.states))
	for _, state := range s.states[1:] {
		updates <- state
	}
	close(updates)
	return updates, nil
}

func (s *streamingJobs) PodLogs(_ context.Context, _ string, _ bool, _ int64) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(s.log)), nil
}

// collect runs a stream and returns the events in the order they were sent.
func collect(t *testing.T, runner jobs) []string {
	t.Helper()
	service := newDeploymentServiceWithJobs(newFakeRepo(), newFakeStore(), runner)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	var events []string
	err := service.StreamJob(ctx, "helm-rollout-abcde", 100,
		func(event string, payload JobEvent) error {
			if event == "" {
				return nil // a heartbeat is not part of the sequence
			}
			line := event
			if payload.Phase != "" {
				line += ":" + payload.Phase
			}
			if payload.Line != "" {
				line += ":" + payload.Line
			}
			events = append(events, line)
			return nil
		})
	if err != nil {
		t.Fatalf("StreamJob: %v", err)
	}
	return events
}

// The snapshot comes first on every connection, and it is what makes a reconnect
// need no server-side memory: the present truth arrives before anything
// incremental. A stream that started with a phase change would leave a browser
// that just reconnected with no idea what it had missed.
func TestStreamJobOpensWithASnapshot(t *testing.T) {
	events := collect(t, &streamingJobs{
		states: []Job{
			{Phase: PhasePending},
			{Phase: PhaseRunning, Pod: "helm-rollout-abcde-x7k2q"},
			{Phase: PhaseSucceeded, Pod: "helm-rollout-abcde-x7k2q"},
		},
		log: "one\ntwo\n",
	})

	if len(events) == 0 || !strings.HasPrefix(events[0], EventSnapshot) {
		t.Fatalf("the stream should open with a snapshot, and opens with %v", events)
	}
}

// The reason a deploy failed is in the log, so the log has to arrive above the
// word "failed" rather than below it. Two streams would have no defined
// interleaving here; one has exactly this order.
func TestStreamJobDrainsTheLogBeforeSayingItIsDone(t *testing.T) {
	events := collect(t, &streamingJobs{
		states: []Job{
			{Phase: PhaseRunning, Pod: "p"},
			{Phase: PhaseFailed, Pod: "p", Reason: "BackoffLimitExceeded"},
		},
		log: "Error: timed out waiting for the condition\n",
	})

	last := events[len(events)-1]
	if !strings.HasPrefix(last, EventDone) {
		t.Fatalf("the last event should be done, and is %q (all: %v)", last, events)
	}
	logIndex := -1
	for i, event := range events {
		if strings.HasPrefix(event, EventLog) {
			logIndex = i
		}
	}
	if logIndex == -1 {
		t.Fatalf("the log should have been sent, and events were %v", events)
	}
	if logIndex > len(events)-2 {
		t.Errorf("the log should arrive before done, and events were %v", events)
	}
}

// A job that was already over when the browser arrived is the normal case after a
// self-upgrade: the panel comes back once the new pods are ready, and the
// operation it was watching has finished. It must replay and close, not watch
// something that will never change again.
func TestStreamJobReplaysAJobThatAlreadyFinished(t *testing.T) {
	events := collect(t, &streamingJobs{
		states: []Job{{Phase: PhaseSucceeded, Pod: "p"}},
		log:    "Upgrade complete\n",
	})

	if !strings.HasPrefix(events[0], EventSnapshot) {
		t.Errorf("want a snapshot first, got %v", events)
	}
	if last := events[len(events)-1]; last != EventDone+":"+PhaseSucceeded {
		t.Errorf("want done:succeeded last, got %q (all: %v)", last, events)
	}
}

// THE distinction. A watch that ends before the job does means an API pod
// restarted or the connection dropped — which happens on every self-upgrade. It
// is the stream failing, not the deploy, and a client told "done: failed" here
// would report a successful upgrade as a broken one.
func TestStreamJobReportsALostWatchAsAnErrorAndNotAFailure(t *testing.T) {
	events := collect(t, &streamingJobs{
		states: []Job{{Phase: PhaseRunning, Pod: "p"}},
		log:    "",
	})

	last := events[len(events)-1]
	if !strings.HasPrefix(last, EventError) {
		t.Errorf("a lost watch should be an error event, got %q (all: %v)", last, events)
	}
	for _, event := range events {
		if strings.HasPrefix(event, EventDone) {
			t.Errorf("a lost watch must never be reported as done: %v", events)
		}
	}
}
