package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// The events a schedule stream sends.
const (
	// EventState carries the schedule and its runs, whole. Sent on connecting and
	// again whenever either has changed.
	EventState = "state"
	// EventError means the STREAM failed, not a backup.
	EventError = "error"
)

// How often the stream looks at the cluster, and how often it says it is alive
// when nothing has changed.
//
// Looking rather than watching, deliberately. A schedule page wants two things
// that come from two kinds of object — the CronJob's last success and its Jobs'
// phases — and a pair of watches with their resourceVersions and re-establishing
// is a lot of machinery for a page somebody has open for a minute. Two list calls
// every two seconds per open tab is nothing to the API server, and what goes over
// the wire is still only the changes.
const (
	streamInterval  = 2 * time.Second
	streamHeartbeat = 15 * time.Second
)

// ScheduleState is one schedule as its page shows it, live.
type ScheduleState struct {
	Schedule Schedule `json:"schedule"`
	Runs     []Run    `json:"runs"`
}

// StreamSchedule sends a schedule's state on connecting and on every change,
// until the context ends.
//
// emit receives EventState with the state, or an empty event name for a
// heartbeat; an error from it is the client hanging up and ends the stream.
// A failed read ends the stream too, with the error: the client reconnects, and a
// schedule that was deleted meanwhile answers that reconnection with a 404 rather
// than with a stream that goes quiet forever.
func (s *Service) StreamSchedule(ctx context.Context, name string,
	emit func(event string, state ScheduleState) error,
) error {
	return s.streamSchedule(ctx, name, streamInterval, streamHeartbeat, emit)
}

func (s *Service) streamSchedule(ctx context.Context, name string, interval, heartbeat time.Duration,
	emit func(event string, state ScheduleState) error,
) error {
	if err := validateName(name); err != nil {
		return err
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	tracker := &stateTracker{heartbeat: heartbeat, quietSince: time.Now()}
	for {
		state, err := s.readState(ctx, name)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if err := tracker.observe(name, state, emit); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// stateTracker decides, one reading at a time, whether the client hears about
// it: a state it has not seen is sent, and a long enough silence earns a
// heartbeat so a proxy between here and the browser does not close it as idle.
type stateTracker struct {
	heartbeat  time.Duration
	last       []byte
	quietSince time.Time
}

func (t *stateTracker) observe(name string, state ScheduleState,
	emit func(event string, state ScheduleState) error,
) error {
	encoded, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode the state of %s: %w", name, err)
	}

	switch {
	case t.last == nil || string(encoded) != string(t.last):
		if err := emit(EventState, state); err != nil {
			return err
		}
		t.last = encoded
	case time.Since(t.quietSince) >= t.heartbeat:
		if err := emit("", ScheduleState{}); err != nil {
			return err
		}
	default:
		return nil
	}
	t.quietSince = time.Now()
	return nil
}

func (s *Service) readState(ctx context.Context, name string) (ScheduleState, error) {
	schedule, err := s.schedules.ReadSchedule(ctx, name)
	if err != nil {
		return ScheduleState{}, err
	}
	runs, err := s.schedules.ListRuns(ctx, name)
	if err != nil {
		return ScheduleState{}, err
	}
	// A list the page walks, never null.
	if runs == nil {
		runs = []Run{}
	}
	return ScheduleState{Schedule: schedule, Runs: runs}, nil
}

// errStop ends a stream in a test without being mistaken for a failure.
var errStop = errors.New("stop")
