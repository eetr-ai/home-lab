package backup

import (
	"context"
	"errors"
	"testing"
	"time"
)

// countingSchedules changes what it reports after a number of reads, the way a
// run moving from running to succeeded would.
type countingSchedules struct {
	*fakeSchedules
	reads    int
	changeAt int
}

func (c *countingSchedules) ListRuns(context.Context, string) ([]Run, error) {
	c.reads++
	if c.reads >= c.changeAt {
		return []Run{{Name: "run", Phase: PhaseSucceeded}}, nil
	}
	return []Run{{Name: "run", Phase: PhaseRunning}}, nil
}

// The stream sends the state once on connecting and then only when it changes:
// six reads of which only two differ must be two events, not six.
func TestTheStreamSendsOnConnectingAndOnChangeOnly(t *testing.T) {
	schedules := &countingSchedules{fakeSchedules: newFakeSchedules(), changeAt: 4}
	schedules.schedules["nightly"] = validSchedule()
	service := newScheduleService(schedules, allEngines)

	// Heartbeats on every quiet tick, so the callback keeps being called after
	// the state stops changing and the test can stop it.
	var phases []string
	err := service.streamSchedule(context.Background(), "nightly", time.Millisecond, time.Millisecond,
		func(event string, state ScheduleState) error {
			if event == EventState {
				phases = append(phases, state.Runs[0].Phase)
			}
			if schedules.reads >= 6 {
				return errStop
			}
			return nil
		})

	if !errors.Is(err, errStop) {
		t.Fatalf("expected the test's own stop, got %v", err)
	}
	if len(phases) != 2 || phases[0] != PhaseRunning || phases[1] != PhaseSucceeded {
		t.Errorf("expected one event per distinct state, got %v", phases)
	}
}

func TestAQuietStreamStillSaysItIsAlive(t *testing.T) {
	schedules := newFakeSchedules()
	schedules.schedules["nightly"] = validSchedule()
	service := newScheduleService(schedules, allEngines)

	heartbeats := 0
	_ = service.streamSchedule(context.Background(), "nightly", time.Millisecond, time.Millisecond,
		func(event string, _ ScheduleState) error {
			if event == "" {
				heartbeats++
			}
			if heartbeats == 2 {
				return errStop
			}
			return nil
		})
	if heartbeats != 2 {
		t.Errorf("expected heartbeats while nothing changes, got %d", heartbeats)
	}
}

// A schedule deleted mid-stream ends it with the error, so the client's
// reconnection meets a 404 rather than a stream that goes silent.
func TestAScheduleThatDisappearsEndsTheStream(t *testing.T) {
	schedules := newFakeSchedules()
	schedules.schedules["nightly"] = validSchedule()
	service := newScheduleService(schedules, allEngines)

	err := service.streamSchedule(context.Background(), "nightly", time.Millisecond, time.Hour,
		func(string, ScheduleState) error {
			delete(schedules.schedules, "nightly")
			return nil
		})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected not found, got %v", err)
	}
}

func TestACancelledStreamEndsQuietly(t *testing.T) {
	schedules := newFakeSchedules()
	schedules.schedules["nightly"] = validSchedule()
	ctx, cancel := context.WithCancel(context.Background())
	service := newScheduleService(schedules, allEngines)

	err := service.streamSchedule(ctx, "nightly", time.Millisecond, time.Hour,
		func(string, ScheduleState) error {
			cancel()
			return nil
		})
	if err != nil {
		t.Errorf("a client leaving is not an error, got %v", err)
	}
}
