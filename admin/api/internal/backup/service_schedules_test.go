package backup

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeSchedules keeps schedules in a map the way the CronJobs would.
type fakeSchedules struct {
	schedules map[string]ScheduleRequest
	created   []string
	err       error
}

func newFakeSchedules() *fakeSchedules {
	return &fakeSchedules{schedules: map[string]ScheduleRequest{}}
}

func (f *fakeSchedules) ListSchedules(context.Context) ([]Schedule, error) {
	ret := make([]Schedule, 0, len(f.schedules))
	for _, request := range f.schedules {
		ret = append(ret, Schedule{ScheduleRequest: request})
	}
	return ret, f.err
}

func (f *fakeSchedules) ReadSchedule(_ context.Context, name string) (Schedule, error) {
	request, ok := f.schedules[name]
	if !ok {
		return Schedule{}, ErrNotFound
	}
	return Schedule{ScheduleRequest: request}, f.err
}

func (f *fakeSchedules) CreateSchedule(_ context.Context, request ScheduleRequest, _ string) error {
	if _, ok := f.schedules[request.Name]; ok {
		return ErrAlreadyExists
	}
	f.created = append(f.created, request.Name)
	f.schedules[request.Name] = request
	return f.err
}

func (f *fakeSchedules) UpdateSchedule(_ context.Context, request ScheduleRequest, _ string) error {
	if _, ok := f.schedules[request.Name]; !ok {
		return ErrNotFound
	}
	f.schedules[request.Name] = request
	return f.err
}

func (f *fakeSchedules) DeleteSchedule(_ context.Context, name string) error {
	delete(f.schedules, name)
	return f.err
}

func (f *fakeSchedules) RunSchedule(_ context.Context, name, _ string) (Run, error) {
	return Run{Name: name + "-run", Manual: true}, f.err
}

func (f *fakeSchedules) ListRuns(context.Context, string) ([]Run, error) { return nil, f.err }

func validSchedule() ScheduleRequest {
	return ScheduleRequest{
		Name:      "nightly",
		Cron:      "0 3 * * *",
		TimeZone:  "America/Los_Angeles",
		Databases: []DatabaseRef{{Engine: EnginePostgres, Name: "app"}, {Engine: EngineMongo, Name: "events"}},
		Folder:    Folder{ID: "1AbCdEfGhIjKlMnOp", Name: "Lab backups"},
		Keep:      7,
	}
}

func TestScheduleValidationRefusesBeforeTheCluster(t *testing.T) {
	tests := []struct {
		name    string
		change  func(*ScheduleRequest)
		message string
	}{
		{"uppercase name", func(r *ScheduleRequest) { r.Name = "Nightly" }, "DNS label"},
		{"long name", func(r *ScheduleRequest) { r.Name = strings.Repeat("a", maxScheduleName+1) }, "DNS label"},
		{"no cron", func(r *ScheduleRequest) { r.Cron = " " }, "cron"},
		{"made-up zone", func(r *ScheduleRequest) { r.TimeZone = "Mars/Olympus" }, "time zone"},
		// Local is the controller's zone, whatever that is, which is not a choice.
		{"local zone", func(r *ScheduleRequest) { r.TimeZone = "Local" }, "time zone"},
		{"keep none", func(r *ScheduleRequest) { r.Keep = 0 }, "keep"},
		{"keep too many", func(r *ScheduleRequest) { r.Keep = maxKeep + 1 }, "keep"},
		{"no folder", func(r *ScheduleRequest) { r.Folder.ID = "" }, "folder"},
		{"folder id with a quote", func(r *ScheduleRequest) { r.Folder.ID = "abcdefghij' or 1=1" }, "folder"},
		{"no databases", func(r *ScheduleRequest) { r.Databases = nil }, "databases"},
		{"unknown engine", func(r *ScheduleRequest) { r.Databases[0].Engine = "mysql" }, "engine"},
		// The names reach a shell, a file name and a Drive query, so an allowlist
		// is what keeps all three safe without each of them escaping.
		{"shell in a name", func(r *ScheduleRequest) { r.Databases[0].Name = "app;rm -rf /" }, "database name"},
		{"globals by hand", func(r *ScheduleRequest) { r.Databases[0].Name = globalsDatabase }, "database name"},
		{"listed twice", func(r *ScheduleRequest) { r.Databases[1] = r.Databases[0] }, "twice"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schedules := newFakeSchedules()
			service := newScheduleService(schedules, allEngines)
			request := validSchedule()
			test.change(&request)

			_, err := service.CreateSchedule(context.Background(), request, "op")
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), test.message) {
				t.Errorf("expected an invalid request about %q, got %v", test.message, err)
			}
			if len(schedules.created) != 0 {
				t.Error("nothing should have reached the cluster")
			}
		})
	}
}

// A schedule naming a server this lab has no credential for would create a
// CronJob whose pods can never start.
func TestAScheduleCannotNameAServerTheLabDoesNotManage(t *testing.T) {
	service := newScheduleService(newFakeSchedules(), map[string]bool{EnginePostgres: true})
	_, err := service.CreateSchedule(context.Background(), validSchedule(), "op")
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "does not manage mongo") {
		t.Errorf("expected a refusal naming mongo, got %v", err)
	}
}

func TestCreatingAScheduleNeedsAnOAuthClient(t *testing.T) {
	service := NewService(Dependencies{Schedules: newFakeSchedules(), Engines: allEngines, Logger: discard()})
	if _, err := service.CreateSchedule(context.Background(), validSchedule(), "op"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("expected not configured, got %v", err)
	}
}

// The path names the schedule on replace. A body naming another one must not
// turn an edit of one schedule into an edit of a different one.
func TestUpdateTakesTheNameFromThePath(t *testing.T) {
	schedules := newFakeSchedules()
	schedules.schedules["nightly"] = validSchedule()
	service := newScheduleService(schedules, allEngines)

	request := validSchedule()
	request.Name = "someone-else"
	request.Keep = 3
	updated, err := service.UpdateSchedule(context.Background(), "nightly", request, "op")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "nightly" || schedules.schedules["nightly"].Keep != 3 {
		t.Errorf("the path's schedule should be the one updated, got %+v", updated)
	}
	if _, exists := schedules.schedules["someone-else"]; exists {
		t.Error("the body's name must not create or touch another schedule")
	}
}

func TestPathNamesAreValidatedBeforeTheCluster(t *testing.T) {
	service := newScheduleService(newFakeSchedules(), allEngines)
	for _, name := range []string{"", "Upper", "a/b", "../x"} {
		if _, err := service.RunSchedule(context.Background(), name, "op"); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: expected an invalid name, got %v", name, err)
		}
	}
}

func TestListFilesReadsTheSchedulesOwnFolder(t *testing.T) {
	schedules := newFakeSchedules()
	schedules.schedules["nightly"] = validSchedule()
	google := &fakeGoogle{files: []File{{Name: "nightly-postgres-app.dump"}}}
	service := NewService(Dependencies{
		Google: google, Grants: &fakeGrants{grant: &Grant{RefreshToken: "r"}}, Schedules: schedules,
		Config: testGoogleConfig, Engines: allEngines, Logger: discard(),
	})

	files, err := service.ListFiles(context.Background(), "nightly")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || google.listedFolder != validSchedule().Folder.ID {
		t.Errorf("expected the schedule's folder to be listed, listed %q", google.listedFolder)
	}
}

var allEngines = map[string]bool{EnginePostgres: true, EngineMongo: true}

func newScheduleService(schedules ScheduleStore, engines map[string]bool) *Service {
	return NewService(Dependencies{
		Google: &fakeGoogle{}, Grants: &fakeGrants{}, Schedules: schedules,
		Config: testGoogleConfig, Engines: engines, Logger: discard(),
	})
}
