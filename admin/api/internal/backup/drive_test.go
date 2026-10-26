package backup

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func pointDriveAt(t *testing.T, server *httptest.Server) {
	t.Helper()
	files, upload := driveFilesURL, driveUploadURL
	driveFilesURL, driveUploadURL = server.URL+"/files", server.URL+"/upload"
	t.Cleanup(func() { driveFilesURL, driveUploadURL = files, upload })
}

// The resumable upload is two requests, and what this asserts is the half Drive
// cannot be asked about later: the metadata, including the tags retention selects
// on, and that the body sent is the file.
func TestUploadOpensASessionAndSendsTheFile(t *testing.T) {
	var metadata map[string]any
	var body string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/upload":
			_ = json.NewDecoder(r.Body).Decode(&metadata)
			if r.URL.Query().Get("uploadType") != "resumable" || r.Header.Get("X-Upload-Content-Length") != "4" {
				t.Errorf("not a resumable upload of 4 bytes: %s %v", r.URL, r.Header)
			}
			w.Header().Set("Location", server.URL+"/session")
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodPut && r.URL.Path == "/session":
			raw, _ := io.ReadAll(r.Body)
			body = string(raw)
			_, _ = w.Write([]byte(`{"id":"f1","name":"n","size":"4","appProperties":{"homeLabDatabase":"postgres/app"}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
		}
	}))
	defer server.Close()
	pointDriveAt(t, server)

	client := &DriveClient{httpClient: server.Client()}
	file, err := client.Upload(context.Background(), "folder", "n", "nightly", "postgres/app",
		strings.NewReader("dump"), 4)
	if err != nil {
		t.Fatal(err)
	}

	if body != "dump" || file.ID != "f1" || file.Size != 4 || file.Database != "postgres/app" {
		t.Errorf("unexpected upload: body %q, file %+v", body, file)
	}
	properties, _ := metadata["appProperties"].(map[string]any)
	if properties[propertySchedule] != "nightly" || properties[propertyDatabase] != "postgres/app" {
		t.Errorf("the upload must carry the tags retention selects on, got %v", metadata)
	}
	if parents, _ := metadata["parents"].([]any); len(parents) != 1 || parents[0] != "folder" {
		t.Errorf("the upload must land in the schedule's folder, got %v", metadata["parents"])
	}
}

// Retention trusts the listing to hold only this schedule's untrashed files, so
// the query is the safety property, and it is read here rather than assumed.
func TestListAsksForTheSchedulesUntrashedFilesAndFollowsPages(t *testing.T) {
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("q"))
		if r.URL.Query().Get("pageToken") == "" {
			_, _ = w.Write([]byte(`{"nextPageToken":"two","files":[{"id":"a"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"files":[{"id":"b"}]}`))
	}))
	defer server.Close()
	pointDriveAt(t, server)

	files, err := (&DriveClient{httpClient: server.Client()}).List(context.Background(), "folder", "nightly")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || len(queries) != 2 {
		t.Fatalf("expected two pages and two files, got %d files over %d requests", len(files), len(queries))
	}
	for _, part := range []string{"'folder' in parents", "trashed = false",
		"key='homeLabSchedule' and value='nightly'"} {
		if !strings.Contains(queries[0], part) {
			t.Errorf("the query %q is missing %q", queries[0], part)
		}
	}
}

func TestTrashMovesToTheTrashAndDoesNotDelete(t *testing.T) {
	var method, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		_, _ = w.Write([]byte(`{"id":"x"}`))
	}))
	defer server.Close()
	pointDriveAt(t, server)

	if err := (&DriveClient{httpClient: server.Client()}).Trash(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPatch || body != `{"trashed":true}` {
		t.Errorf("expected a PATCH to trashed, got %s %s", method, body)
	}
}

func TestDriveRefusalsKeepDrivesOwnSentence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"File not found: folder."}}`))
	}))
	defer server.Close()
	pointDriveAt(t, server)

	_, err := (&DriveClient{httpClient: server.Client()}).List(context.Background(), "folder", "nightly")
	if !errors.Is(err, ErrGoogleRefused) || !strings.Contains(err.Error(), "File not found") {
		t.Errorf("expected Drive's own refusal, got %v", err)
	}
}

// A 429 or a 5xx is Drive busy or failing. Calling it a refusal would tell the
// operator to reconnect an account that is fine.
func TestDriveBeingBusyIsUnreachableNotARefusal(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		pointDriveAt(t, server)

		_, err := (&DriveClient{httpClient: server.Client()}).List(context.Background(), "folder", "nightly")
		server.Close()
		if !errors.Is(err, ErrGoogleUnreachable) || errors.Is(err, ErrGoogleRefused) {
			t.Errorf("status %d: expected unreachable, got %v", status, err)
		}
	}
}

// An empty folder must encode as [] — the panel iterates it, and null is what
// crashed the schedule page on a folder with nothing in it yet.
func TestAnEmptyFolderListsAsAnEmptyArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"files":[]}`))
	}))
	defer server.Close()
	pointDriveAt(t, server)

	files, err := (&DriveClient{httpClient: server.Client()}).List(context.Background(), "folder", "nightly")
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(files); string(raw) != "[]" {
		t.Errorf("an empty folder must encode as [], got %s", raw)
	}
}
