package http

import (
	"log/slog"
	"net/http"
	"time"
)

// MaxStreamDuration caps a single streamed response.
//
// A forgotten browser tab holds a goroutine here and a connection to the cluster
// for as long as it is open, and neither side has any reason to notice. This is
// what ends it.
const MaxStreamDuration = 30 * time.Minute

// ClearWriteDeadline lifts the server's write timeout for this one response, and
// reports whether streaming may proceed.
//
// The server gives every response thirty seconds to be written, which is right
// for JSON and wrong for a tail. Clearing it is scoped to this request: net/http
// sets the deadline per request and reinstalls it before reading the next one off
// the same connection, so nothing else is weakened.
//
// The read deadline needs no such treatment, but only because the routes that
// stream are bodyless GETs — net/http clears that one itself before dispatching
// such a handler. Making one a POST later would put the deadline back in play
// with nothing to catch it.
//
// A failure here is only reachable if something has begun wrapping the
// ResponseWriter, in which case the stream would die at thirty seconds with no
// explanation. Better to refuse than to serve that.
//
// It lives here rather than beside either caller because two slices now stream —
// the Kubernetes slice's pod logs and the Helm slice's job progress — and a slice
// never imports another slice's internals.
func ClearWriteDeadline(w http.ResponseWriter, label string) bool {
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		slog.Error("cannot clear the write deadline for a stream",
			slog.String("stream", label), slog.Any("error", err))
		Error(w, http.StatusInternalServerError, "internal_error",
			"streaming is not available on this server")
		return false
	}
	return true
}
