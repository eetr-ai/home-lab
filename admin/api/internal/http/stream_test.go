package http

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The server bounds how long a response may take to write, which is right for
// JSON and fatal for a tail. These tests are the evidence that clearing the
// deadline for one route works, and that not clearing it does not — because
// without the second half the first proves nothing.
//
// The real server allows thirty seconds. These use a very short one so the whole
// thing runs in milliseconds; the mechanism under test is identical.
const (
	testWriteTimeout = 150 * time.Millisecond
	// Comfortably past the deadline, so a stream that was going to be cut has
	// certainly been cut by the time the second line is expected.
	pastTheDeadline = 3 * testWriteTimeout
)

// slowLines writes one line, waits past the write deadline, then writes another.
//
// An io.Reader rather than a channel because that is what StreamText consumes,
// and a pod producing an occasional line is exactly this shape.
type slowLines struct {
	sent  int
	pause time.Duration
}

func (s *slowLines) Read(p []byte) (int, error) {
	switch s.sent {
	case 0:
		s.sent++
		return copy(p, "first\n"), nil
	case 1:
		time.Sleep(s.pause)
		s.sent++
		return copy(p, "second\n"), nil
	default:
		return 0, io.EOF
	}
}

// serve runs one handler on a server with a short write timeout and returns a
// reader over the response body.
func serve(t *testing.T, handler http.HandlerFunc) *bufio.Reader {
	t.Helper()

	server := httptest.NewUnstartedServer(handler)
	server.Config.WriteTimeout = testWriteTimeout
	server.Start()
	t.Cleanup(server.Close)

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("build the request: %v", err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("open the stream: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	return bufio.NewReader(response.Body)
}

// The whole point of the deadline handling. A follow stream is quiet for long
// stretches, and being cut mid-tail looks from the browser like the pod simply
// stopped logging.
func TestStreamSurvivesTheWriteDeadlineWhenItIsCleared(t *testing.T) {
	body := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
			t.Errorf("clear the write deadline: %v", err)
			return
		}
		StreamText(w, r, &slowLines{pause: pastTheDeadline}, "test stream")
	})

	for _, want := range []string{"first\n", "second\n"} {
		line, err := body.ReadString('\n')
		if err != nil {
			t.Fatalf("read %q: %v", want, err)
		}
		if line != want {
			t.Fatalf("line = %q, want %q", line, want)
		}
	}
}

// The control. Without this the test above could pass because the deadline never
// applied in the first place, and would keep passing if the clearing were
// deleted.
func TestStreamIsCutAtTheWriteDeadlineWithoutIt(t *testing.T) {
	body := serve(t, func(w http.ResponseWriter, r *http.Request) {
		StreamText(w, r, &slowLines{pause: pastTheDeadline}, "test stream")
	})

	if line, err := body.ReadString('\n'); err != nil || line != "first\n" {
		t.Fatalf("first line = %q, %v; want %q", line, err, "first\n")
	}
	if _, err := body.ReadString('\n'); err == nil {
		t.Fatal("the second line arrived; the write deadline was expected to cut the stream")
	}
}

// The header goes out before any body so an idle follow reads as a live 200. A
// client that cannot tell "connected, waiting" from "hung" shows a spinner
// forever on a pod that is simply quiet.
func TestHeaderIsFlushedBeforeTheFirstLine(t *testing.T) {
	// Released with defer rather than t.Cleanup: cleanups run last-registered
	// first, so the server's Close would run before it — and Close waits for the
	// handler, which is blocked on this channel. That is a deadlock, not a test.
	release := make(chan struct{})
	defer close(release)

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
			t.Errorf("clear the write deadline: %v", err)
			return
		}
		StreamText(w, r, blockingReader{release}, "test stream")
	}))
	server.Config.WriteTimeout = testWriteTimeout
	server.Start()
	t.Cleanup(server.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("build the request: %v", err)
	}

	// This returns only once the header has arrived. If it were withheld until the
	// first byte of body, this would block until the context expired.
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("the header was not flushed before the body: %v", err)
	}
	defer func() { _ = response.Body.Close() }()

	if got := response.Header.Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := response.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("X-Accel-Buffering = %q, want no — a proxy would batch the tail", got)
	}
}

// blockingReader produces nothing until it is released, standing in for a pod
// that has not logged anything yet.
type blockingReader struct{ release <-chan struct{} }

func (b blockingReader) Read([]byte) (int, error) {
	<-b.release
	return 0, io.EOF
}
