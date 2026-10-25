package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

// fakeFiles is a Drive folder in memory.
type fakeFiles struct {
	existing  []File
	uploaded  []string
	trashed   []string
	failNames map[string]bool
	listErr   error
}

func (f *fakeFiles) Upload(_ context.Context, _, name, _, database string, body io.Reader, _ int64) (File, error) {
	if _, err := io.ReadAll(body); err != nil {
		return File{}, fmt.Errorf("read the body: %w", err)
	}
	if f.failNames[database] {
		return File{}, ErrGoogleRefused
	}
	f.uploaded = append(f.uploaded, name)
	f.existing = append(f.existing, File{ID: name, Name: name, Database: database, CreatedAt: time.Now()})
	return File{Name: name}, nil
}

func (f *fakeFiles) List(context.Context, string, string) ([]File, error) {
	return f.existing, f.listErr
}

func (f *fakeFiles) Trash(_ context.Context, id string) error {
	f.trashed = append(f.trashed, id)
	return nil
}

func workWith(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("dump"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func old(database string, age int) File {
	return File{
		ID: fmt.Sprintf("%s-%d", database, age), Name: fmt.Sprintf("%s-%d", database, age),
		Database: database, CreatedAt: time.Now().Add(-time.Duration(age) * 24 * time.Hour),
	}
}

var runTime = time.Date(2026, 10, 25, 3, 0, 0, 0, time.UTC)

func TestARunUploadsEveryDumpUnderItsScheduleAndTime(t *testing.T) {
	dir := workWith(t, "postgres__app.dump", "postgres__@globals.sql.gz", "mongo__events.archive.gz",
		".bash_history", "notes.txt")
	files := &fakeFiles{}

	err := uploadRun(context.Background(), files, UploadSpec{Schedule: "nightly", FolderID: "f", Keep: 3, Dir: dir},
		runTime, discard())
	if err != nil {
		t.Fatal(err)
	}

	sort.Strings(files.uploaded)
	want := []string{
		"nightly-mongo-events-20261025T030000Z.archive.gz",
		"nightly-postgres-@globals-20261025T030000Z.sql.gz",
		"nightly-postgres-app-20261025T030000Z.dump",
	}
	if !reflect.DeepEqual(files.uploaded, want) {
		t.Errorf("uploaded %v, want %v (and nothing the scripts did not write)", files.uploaded, want)
	}
}

func TestARunKeepsTheNewestNOfEachDatabase(t *testing.T) {
	dir := workWith(t, "postgres__app.dump", "mongo__events.archive.gz")
	files := &fakeFiles{existing: []File{
		old("postgres/app", 1), old("postgres/app", 2), old("postgres/app", 3),
		old("mongo/events", 1),
	}}

	err := uploadRun(context.Background(), files, UploadSpec{Schedule: "nightly", FolderID: "f", Keep: 2, Dir: dir},
		runTime, discard())
	if err != nil {
		t.Fatal(err)
	}

	// Keep 2 of postgres/app: tonight's and the 1-day-old one. Mongo has two in
	// all, so nothing of it goes.
	sort.Strings(files.trashed)
	if want := []string{"postgres/app-2", "postgres/app-3"}; !reflect.DeepEqual(files.trashed, want) {
		t.Errorf("trashed %v, want %v", files.trashed, want)
	}
}

// The rule that matters most: tonight failing must not cost the backups that
// were good. A database whose dump or upload failed is not pruned at all.
func TestAFailedDatabaseKeepsAllItsHistoryAndFailsTheRun(t *testing.T) {
	tests := []struct {
		name  string
		work  []string
		files *fakeFiles
	}{
		{"dump failed", []string{"postgres__app.failed"}, &fakeFiles{}},
		{"upload failed", []string{"postgres__app.dump"},
			&fakeFiles{failNames: map[string]bool{"postgres/app": true}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := workWith(t, append(test.work, "mongo__events.archive.gz")...)
			test.files.existing = []File{old("postgres/app", 1), old("postgres/app", 2), old("mongo/events", 1),
				old("mongo/events", 2)}

			err := uploadRun(context.Background(), test.files,
				UploadSpec{Schedule: "nightly", FolderID: "f", Keep: 1, Dir: dir}, runTime, discard())
			if err == nil {
				t.Error("the run should fail, so the Job reads failed")
			}
			for _, trashed := range test.files.trashed {
				if trashed == "postgres/app-1" || trashed == "postgres/app-2" {
					t.Errorf("%s was trashed although tonight's postgres/app did not make it", trashed)
				}
			}
			if len(test.files.trashed) == 0 {
				t.Error("the database that did succeed should still be pruned")
			}
		})
	}
}

func TestAFailedListingTrashesNothing(t *testing.T) {
	dir := workWith(t, "postgres__app.dump")
	files := &fakeFiles{listErr: ErrGoogleUnreachable}

	err := uploadRun(context.Background(), files, UploadSpec{Schedule: "nightly", FolderID: "f", Keep: 1, Dir: dir},
		runTime, discard())
	if !errors.Is(err, ErrGoogleUnreachable) || len(files.trashed) != 0 {
		t.Errorf("expected the listing failure and no trashing, got %v, %v", err, files.trashed)
	}
}

func TestToTrashOnlyEverConsidersTaggedFilesOfSucceededDatabases(t *testing.T) {
	files := []File{
		old("postgres/app", 1), old("postgres/app", 2),
		// An untagged file in the same folder has no database, and is never a
		// candidate whatever its name says.
		{ID: "mine", Name: "nightly-postgres-app-old.dump", CreatedAt: time.Now().Add(-1000 * time.Hour)},
	}
	got := toTrash(files, 1, map[string]bool{"postgres/app": true})
	if len(got) != 1 || got[0].ID != "postgres/app-2" {
		t.Errorf("expected only the older tagged backup, got %+v", got)
	}
}

func TestToTrashBreaksTiesOnTheName(t *testing.T) {
	at := time.Now()
	files := []File{
		{ID: "a", Name: "nightly-postgres-app-20261024T030000Z.dump", Database: "postgres/app", CreatedAt: at},
		{ID: "b", Name: "nightly-postgres-app-20261025T030000Z.dump", Database: "postgres/app", CreatedAt: at},
	}
	got := toTrash(files, 1, map[string]bool{"postgres/app": true})
	if len(got) != 1 || got[0].ID != "a" {
		t.Errorf("with equal times the older name should go, got %+v", got)
	}
}

func TestTheUploaderSaysWhatIsMissing(t *testing.T) {
	args := UploadSpec{Schedule: "nightly", FolderID: "1AbCdEfGhIjKlMnOp", Keep: 1, Dir: t.TempDir()}.Args()[1:]

	t.Setenv("ADMIN_GOOGLE_CLIENT_ID", "")
	t.Setenv("ADMIN_GOOGLE_CLIENT_SECRET", "")
	if err := RunUpload(context.Background(), args, discard()); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("expected not configured, got %v", err)
	}

	t.Setenv("ADMIN_GOOGLE_CLIENT_ID", "c")
	t.Setenv("ADMIN_GOOGLE_CLIENT_SECRET", "s")
	t.Setenv("ADMIN_GOOGLE_REFRESH_TOKEN", "")
	if err := RunUpload(context.Background(), args, discard()); !errors.Is(err, ErrNotConnected) {
		t.Errorf("expected not connected, got %v", err)
	}
}

// Green with nothing kept is the worst thing a backup job can say.
func TestARunWithNothingToUploadFails(t *testing.T) {
	dir := workWith(t, ".bash_history")
	files := &fakeFiles{existing: []File{old("postgres/app", 1), old("postgres/app", 2)}}

	err := uploadRun(context.Background(), files, UploadSpec{Schedule: "nightly", FolderID: "f", Keep: 1, Dir: dir},
		runTime, discard())
	if err == nil {
		t.Error("an empty work directory must fail the run")
	}
	if len(files.trashed) != 0 {
		t.Error("nothing may be pruned by a run that uploaded nothing")
	}
}
