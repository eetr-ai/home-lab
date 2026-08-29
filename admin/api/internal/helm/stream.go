package helm

import (
	"bufio"
	"context"
	"errors"
	"io"
	"time"
)

// The events a job stream carries.
const (
	// EventSnapshot is the whole job, sent first on every connection. It is what
	// makes a reconnect need no server-side memory: the present truth arrives
	// before anything incremental does.
	EventSnapshot = "snapshot"
	// EventPhase is a transition.
	EventPhase = "phase"
	// EventLog is one line of the job's log.
	EventLog = "log"
	// EventDone is the operation ending. The server closes after it.
	EventDone = "done"
	// EventError is THE STREAM failing, not the operation. Conflating the two is
	// how a panel tells an operator their deploy failed when what actually
	// happened was that an API pod restarted.
	EventError = "error"
)

// heartbeat is how often an idle stream says it is still there.
//
// Without it, an idle stream is indistinguishable from a dropped one to every
// proxy between here and the browser, and the usual outcome is a connection
// closed sixty seconds into a five-minute deploy. A Helm operation is idle for
// most of its life: it is waiting for pods.
const heartbeat = 20 * time.Second

// JobEvent is one thing that happened, as the browser reads it.
type JobEvent struct {
	// Job is set on a snapshot.
	Job *Job `json:"job,omitempty"`
	// Phase is set on a phase change and on done.
	Phase string `json:"phase,omitempty"`
	// Reason says why a job failed, when Kubernetes named a reason.
	Reason string `json:"reason,omitempty"`
	// Pod is the pod running the operation, once there is one.
	Pod string `json:"pod,omitempty"`
	// Line is one log line.
	Line string `json:"line,omitempty"`
	// Error is why the stream stopped, which is not why the operation did.
	Error string `json:"error,omitempty"`
}

// StreamJob reports one operation's progress until it ends or the caller leaves.
//
// One stream carrying both phase changes and log lines, rather than two. The pod
// does not exist for the first moment of every job, which is the argument *for*
// this rather than against: with one stream the server owns the sequence — say
// pending, wait for a pod, open its log, say done — instead of the browser
// inventing a state machine to discover when a second connection becomes
// possible. Two streams also have no defined interleaving, so a panel could
// render "succeeded" above the last twenty lines of the log it is explaining.
//
// send is called for every event and its error ends the stream: it means the
// client hung up, which is how one of these normally ends, and there is no reason
// to carry on watching the cluster for nobody.
//
// Nothing here is remembered between connections. A reconnect gets a fresh
// snapshot and the tail of the log again, which is idempotent and needs no ids —
// and that is the normal path rather than the exceptional one, because upgrading
// the panel's own chart drops this stream from both ends every time.
func (s *Service) StreamJob(ctx context.Context, name string, tail int64,
	send func(event string, payload JobEvent) error,
) error {
	job, err := s.ReadJob(ctx, name)
	if err != nil {
		return err
	}

	if err := send(EventSnapshot, JobEvent{
		Job: &job, Phase: job.Phase, Reason: job.Reason, Pod: job.Pod,
	}); err != nil {
		return nil
	}

	// A job that was already over when the browser arrived — a page loaded after
	// the operation finished, which is exactly what happens when the panel comes
	// back from being upgraded. Replay the log and say so, rather than watching
	// something that will never change again.
	if !job.Active() {
		s.streamLog(ctx, job, tail, send)
		_ = send(EventDone, JobEvent{Phase: job.Phase, Reason: job.Reason})
		return nil
	}

	updates, err := s.jobs.WatchJob(ctx, name)
	if err != nil {
		return err
	}

	// The log is followed on its own goroutine, because the watch and the log are
	// two blocking reads and this has to interleave them. Both reach the client
	// through the loop below, so the sequence it sees is one order.
	logCtx, stopLog := context.WithCancel(ctx)
	defer stopLog()

	events := make(chan logEvent, eventBuffer)

	// Captured before the goroutine starts, and never read from `job` again. The
	// loop tracks the pod in its own variable: sharing one struct across the two
	// would be a data race, and the race detector finds it.
	startingPod := job.Pod

	go func() {
		defer close(events)
		s.follow(logCtx, name, startingPod, tail, func(event string, payload JobEvent) error {
			select {
			case events <- logEvent{event, payload}:
				return nil
			case <-logCtx.Done():
				return logCtx.Err()
			}
		})
	}()

	return s.watch(ctx, job, updates, events, stopLog, send)
}

// onUpdate handles one watch event, and reports whether the stream is over.
func (s *Service) onUpdate(update Job, open bool, phase, pod *string,
	events chan logEvent, stopLog context.CancelFunc,
	send func(string, JobEvent) error,
) (done bool, err error) {
	if !open {
		// The watch ended without the job reaching a terminal phase. That is the
		// stream failing, not the operation — say so, and let the client
		// reconnect and re-derive.
		return true, send(EventError, JobEvent{
			Phase: *phase,
			Error: "the connection to the cluster ended before the operation did",
		})
	}

	if !announce(update, phase, pod, send) {
		return true, nil
	}
	if !update.Active() {
		return true, s.finish(update, events, stopLog, send)
	}
	return false, nil
}

// announce reports a change in phase or pod, and says whether the client is still
// listening.
//
// Only on an actual change: a watch re-lists on reconnect and re-delivers what it
// already sent, and a browser told "running" four times would render four entries
// for one transition.
func announce(update Job, phase, pod *string, send func(string, JobEvent) error) bool {
	changed := update.Phase != *phase || update.Pod != *pod
	*phase, *pod = update.Phase, update.Pod
	if !changed {
		return true
	}
	return send(EventPhase, JobEvent{
		Phase: update.Phase, Reason: update.Reason, Pod: update.Pod,
	}) == nil
}

// finish closes a stream whose job has reached a terminal phase.
//
// The log drains before the terminal event, so the reason a deploy failed is
// above the word "failed" rather than below it. Not by cancelling the follower
// first: that races, and it loses the race exactly when it matters — a job that
// fails fast reaches its terminal status before its own error message has been
// read, and cancelling then discards the one thing worth showing.
//
// The pod has exited by now, so its log ends on its own and this is normally
// instant. The grace period is for the case where it does not, because a stream
// that never says "done" is worse than one missing its last few lines.
func (s *Service) finish(job Job, events chan logEvent, stopLog context.CancelFunc,
	send func(string, JobEvent) error,
) error {
	s.drain(events, send, logDrainGrace)
	stopLog()
	return send(EventDone, JobEvent{Phase: job.Phase, Reason: job.Reason})
}

// logEvent is one thing the log follower produced, on its way to the client.
type logEvent struct {
	event   string
	payload JobEvent
}

// watch interleaves the job's status changes with its log until it ends.
func (s *Service) watch(ctx context.Context, job Job, updates <-chan Job,
	events chan logEvent, stopLog context.CancelFunc,
	send func(string, JobEvent) error,
) error {
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()

	phase, pod := job.Phase, job.Pod
	for {
		select {
		case <-ctx.Done():
			return nil

		case <-ticker.C:
			// A comment, not an event. The client skips it, and every proxy in
			// between sees traffic.
			if send("", JobEvent{}) != nil {
				return nil
			}

		case entry, open := <-events:
			if !open {
				// The follower is done. Nil the channel so this case stops being
				// ready — a closed one is always ready, and leaving it would spin.
				events = nil
				continue
			}
			if send(entry.event, entry.payload) != nil {
				return nil
			}

		case update, open := <-updates:
			done, err := s.onUpdate(update, open, &phase, &pod, events, stopLog, send)
			if done {
				return err
			}
		}
	}
}

// logDrainGrace bounds the wait for the log to finish after the job has.
const logDrainGrace = 5 * time.Second

// The log scanner's buffer. Helm renders a whole manifest into an error message,
// so a single line can be very long, and a scanner that overflows stops reading —
// which would silently truncate the log exactly when something went wrong.
const (
	logLineBuffer  = 64 * 1024
	logLineMaximum = 1024 * 1024
)

// eventBuffer is how many log lines may be in flight to the writer.
const eventBuffer = 64

// drain forwards what the log goroutine still has, and gives up after grace.
func (s *Service) drain(events chan logEvent, send func(string, JobEvent) error,
	grace time.Duration,
) {
	deadline := time.NewTimer(grace)
	defer deadline.Stop()

	for {
		select {
		case entry, open := <-events:
			if !open {
				return
			}
			if err := send(entry.event, entry.payload); err != nil {
				return
			}
		case <-deadline.C:
			return
		}
	}
}

// follow waits for a pod and then streams its log a line at a time.
//
// The waiting is the reason this exists rather than a call to streamLog: a job's
// pod is not scheduled instantly, and a log opened too early is an error rather
// than an empty stream.
func (s *Service) follow(ctx context.Context, name, pod string, tail int64,
	send func(string, JobEvent) error,
) {
	for pod == "" {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}

		job, err := s.ReadJob(ctx, name)
		if err != nil {
			return
		}
		pod = job.Pod
	}

	s.streamLog(ctx, Job{Name: name, Pod: pod}, tail, send)
}

// streamLog sends a pod's log as one event per line.
//
// A failure to open it is not reported to the client. The usual causes are a pod
// that has not started yet and a pod already reaped, and neither says anything
// about whether the operation succeeded — which the job's own status answers.
func (s *Service) streamLog(ctx context.Context, job Job, tail int64,
	send func(string, JobEvent) error,
) {
	if job.Pod == "" {
		return
	}

	stream, err := s.jobs.PodLogs(ctx, job.Pod, true, tail)
	if err != nil {
		return
	}
	defer func() { _ = stream.Close() }()

	scanner := bufio.NewScanner(stream)
	// Helm renders a whole manifest into an error message, so a line can be long.
	scanner.Buffer(make([]byte, 0, logLineBuffer), logLineMaximum)

	for scanner.Scan() {
		if err := send(EventLog, JobEvent{Line: scanner.Text()}); err != nil {
			return
		}
		if ctx.Err() != nil {
			return
		}
	}
	// A closed stream and a cancelled context are both normal endings here.
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) && ctx.Err() == nil {
		s.logger.Debug("a helm job log stream ended", "job", job.Name, "error", err)
	}
}
