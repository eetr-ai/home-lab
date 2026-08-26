package http

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
)

// streamChunk is the copy buffer for a streamed response. Small on purpose: the
// point is to forward whatever has arrived, not to wait for a buffer to fill.
const streamChunk = 4096

// StreamText copies a reader to the client as chunked plain text, flushing each
// chunk so lines arrive as they are produced.
//
// Four details here are easy to get subtly wrong, which is why this is one
// function rather than something each handler writes:
//
//   - The header is flushed before any body. A follow stream can sit idle before
//     its first line, and net/http or a proxy would otherwise withhold the 200
//     until then — leaving the client unable to tell "connected, waiting" from
//     "hung".
//   - Every chunk is flushed, or the tail is held indefinitely.
//   - A failed write ends the copy silently. The client hung up, which is how a
//     follow stream normally ends.
//   - io.EOF and a cancelled request are both normal endings. Anything else is
//     logged under label.
//
// It writes the status line itself, so the caller must not have written one, and
// it does not close the reader, because the caller owns it.
//
// The caller must also have cleared the write deadline — see the note in the
// Kubernetes slice's log handler. Without that the server's write timeout ends
// the stream partway through, which looks from the client like the pod stopped
// logging.
func StreamText(w http.ResponseWriter, r *http.Request, stream io.Reader, label string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}

	buffer := make([]byte, streamChunk)
	for {
		read, readErr := stream.Read(buffer)
		if read > 0 {
			if _, writeErr := w.Write(buffer[:read]); writeErr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) && r.Context().Err() == nil {
				slog.Error(label, slog.Any("error", readErr))
			}
			return
		}
	}
}
