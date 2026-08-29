package http

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// EventStream writes server-sent events to one client.
//
// The framing is trivial and the three ways to get it wrong are not, which is why
// this is one type rather than something each handler writes:
//
//   - The header must be flushed before any event. A watch can sit idle before its
//     first transition, and net/http or a proxy would otherwise withhold the 200
//     until then — leaving the browser unable to tell "connected, waiting" from
//     "hung".
//   - Every event must be flushed, or the stream arrives in batches.
//   - The payload must not contain a literal newline, or the blank line inside it
//     terminates the frame early and the client reads half an event as a whole
//     one. Nothing splits it into several `data:` lines here because nothing needs
//     to: json.Marshal escapes newlines as \n and emits none of its own. Anything
//     that starts writing pre-encoded or indented JSON through this has to revisit
//     that, and the symptom would be a truncated event rather than an error.
//
// The caller must have cleared the write deadline first — see ClearWriteDeadline.
// Without it the server's write timeout ends the stream partway through, which
// looks from the client like the operation stalled.
type EventStream struct {
	w          http.ResponseWriter
	controller *http.ResponseController
}

// NewEventStream writes the SSE headers and the status line, and returns a stream
// to write events to.
//
// It writes the status line itself, so the caller must not have written one. A
// caller that needs to fail with a status code must do so before calling this —
// after it, there is no status line left to report with.
func NewEventStream(w http.ResponseWriter) *EventStream {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Through a ResponseController rather than a type assertion on http.Flusher,
	// for the reason StreamText gives: a wrapper forwarding Flush only via Unwrap
	// satisfies the controller and fails the assertion.
	controller := http.NewResponseController(w)
	_ = controller.Flush()

	return &EventStream{w: w, controller: controller}
}

// Send writes one named event carrying a JSON payload.
//
// A write error means the client hung up, which is how one of these normally
// ends. It is returned rather than logged so the caller can stop watching the
// cluster rather than carrying on writing into a closed socket.
func (s *EventStream) Send(event string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode the %s event: %w", event, err)
	}

	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, body); err != nil {
		return err
	}
	_ = s.controller.Flush()
	return nil
}

// Comment writes an SSE comment, which a client ignores.
//
// This is the heartbeat. Without one, an idle stream is indistinguishable from a
// dropped one to every proxy between here and the browser, and the usual outcome
// is a connection closed after sixty seconds of a five-minute deploy.
func (s *EventStream) Comment(text string) error {
	if _, err := fmt.Fprintf(s.w, ": %s\n\n", text); err != nil {
		return err
	}
	_ = s.controller.Flush()
	return nil
}
