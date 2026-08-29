package http

import (
	"net/http/httptest"
	"testing"
)

// The framing is the contract with lib/sse.ts on the other side, so it is asserted
// byte for byte rather than by "it contains the payload somewhere".
func TestEventStreamFraming(t *testing.T) {
	tests := []struct {
		name    string
		event   string
		payload any
		want    string
	}{
		{
			name:    "an object becomes one data line",
			event:   "phase",
			payload: map[string]string{"phase": "running"},
			want:    "event: phase\ndata: {\"phase\":\"running\"}\n\n",
		},
		{
			// The one that matters, and the reason the framing can be a single
			// data line: a log line carrying a newline must reach the client with
			// that newline escaped. A literal one would close the frame early and
			// the client would read half an event as a whole one.
			name:    "a newline in the payload is escaped, not framed",
			event:   "log",
			payload: map[string]string{"line": "one\ntwo"},
			want:    "event: log\ndata: {\"line\":\"one\\ntwo\"}\n\n",
		},
		{
			name:    "an empty payload still frames",
			event:   "done",
			payload: struct{}{},
			want:    "event: done\ndata: {}\n\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			stream := NewEventStream(recorder)
			recorder.Body.Reset() // drop anything the header write produced

			if err := stream.Send(test.event, test.payload); err != nil {
				t.Fatalf("Send: %v", err)
			}
			if got := recorder.Body.String(); got != test.want {
				t.Errorf("frame = %q, want %q", got, test.want)
			}
		})
	}
}

// A heartbeat must be a comment, because a client parses it as one and skips it.
// Written as a frame it would arrive as an empty event and be indistinguishable
// from a real one that lost its fields.
func TestEventStreamHeartbeatIsAComment(t *testing.T) {
	recorder := httptest.NewRecorder()
	stream := NewEventStream(recorder)
	recorder.Body.Reset()

	if err := stream.Comment("keep-alive"); err != nil {
		t.Fatalf("Comment: %v", err)
	}
	if got, want := recorder.Body.String(), ": keep-alive\n\n"; got != want {
		t.Errorf("heartbeat = %q, want %q", got, want)
	}
}

// The headers have to be set before anything is written, and the status line has
// to be 200 — a caller that needs a different one must fail before opening this.
func TestEventStreamWritesItsHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewEventStream(recorder)

	if got, want := recorder.Code, 200; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
	for header, want := range map[string]string{
		"Content-Type":      "text/event-stream",
		"X-Accel-Buffering": "no",
	} {
		if got := recorder.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}
