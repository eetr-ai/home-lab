package helm

import (
	"context"
	"fmt"
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
func collect(t *testing.T, runner Jobs) []string {
	t.Helper()
	service := newDeploymentServiceWithJobs(newFakeRepo(), newFakeStore(), runner)

	// Comfortably above logDrainGrace, so a slow drain fails the assertion it is
	// about rather than expiring the context and failing as a timeout.
	ctx, cancel := context.WithTimeout(t.Context(), 3*logDrainGrace)
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

// Draining must not wait on a channel nothing will ever send to.
//
// `watch` nils the events channel once the follower is done, so a closed one
// stops spinning the select. Receiving from a nil channel blocks forever — so a
// drain that waited on one would spend the whole grace period doing nothing
// before every terminal event, on the common path rather than a rare one.
//
// Tested against drain directly: reaching the nil case through StreamJob depends
// on which of two channels the select happens to pick, so a test that went
// through it would pass whether or not the bug was there.
func TestDrainReturnsImmediatelyWhenThereIsNothingToDrain(t *testing.T) {
	service := newDeploymentServiceWithJobs(newFakeRepo(), newFakeStore(), &fakeJobs{})

	for _, test := range []struct {
		name   string
		events chan logEvent
	}{
		{name: "the follower is already done", events: nil},
		{name: "the channel is closed", events: closedEvents()},
	} {
		t.Run(test.name, func(t *testing.T) {
			started := time.Now()
			service.drain(test.events, func(string, JobEvent) error { return nil }, logDrainGrace)

			// Generous: the assertion is "did not wait out the grace", not a
			// benchmark. Anything near the grace means it blocked.
			if elapsed := time.Since(started); elapsed > logDrainGrace/2 {
				t.Errorf("drain took %s, want it to return at once", elapsed)
			}
		})
	}
}

func closedEvents() chan logEvent {
	events := make(chan logEvent)
	close(events)
	return events
}

// logsAfter refuses the log until the Nth attempt, the way the API server refuses
// one for a container that has not started.
type logsAfter struct {
	streamingJobs
	failures int
	attempts int
}

func (l *logsAfter) PodLogs(_ context.Context, _ string, _ bool, _ int64) (io.ReadCloser, error) {
	l.attempts++
	if l.attempts <= l.failures {
		return nil, fmt.Errorf("%w: container helm is waiting to start", ErrNoPodYet)
	}
	return io.NopCloser(strings.NewReader(l.log)), nil
}

// The log has to survive not being readable yet.
//
// Found live, and it made the whole log pane useless: a pod exists almost
// immediately but its container does not, and the API server refuses the log
// until it does. Opening it once and giving up on that error delivered no log at
// all for every operation — the stream carried phases and nothing else.
func TestStreamJobRetriesALogThatIsNotReadableYet(t *testing.T) {
	runner := &logsAfter{
		streamingJobs: streamingJobs{
			states: []Job{
				{Phase: PhaseRunning, Pod: "p"},
				{Phase: PhaseRunning, Pod: "p"},
				{Phase: PhaseSucceeded, Pod: "p"},
			},
			log: "Upgrade complete\n",
		},
		failures: 2,
	}

	events := collect(t, runner)

	var logs int
	for _, event := range events {
		if strings.HasPrefix(event, EventLog) {
			logs++
		}
	}
	if logs == 0 {
		t.Fatalf("the log never arrived after %d attempts; events were %v",
			runner.attempts, events)
	}
	if runner.attempts <= runner.failures {
		t.Errorf("gave up after %d attempts, want it to retry past %d failures",
			runner.attempts, runner.failures)
	}
}
