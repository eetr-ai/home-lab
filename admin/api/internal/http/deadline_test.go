package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A streaming route clears the server's write deadline before writing. If
// something starts wrapping the ResponseWriter so that cannot be done, the
// handler must refuse rather than serve a stream that will be cut at thirty
// seconds with no explanation.
func TestClearWriteDeadlineRefusesAWriterThatCannotSupportIt(t *testing.T) {
	recorder := httptest.NewRecorder()
	// httptest.ResponseRecorder implements neither SetWriteDeadline nor Unwrap,
	// which is exactly the shape being guarded against.
	if ClearWriteDeadline(recorder, "test stream") {
		t.Fatal("ClearWriteDeadline reported success on a writer that cannot support it")
	}
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}
