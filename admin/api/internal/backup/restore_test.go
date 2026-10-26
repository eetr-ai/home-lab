package backup

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func backupFile(database, name string) File {
	return File{ID: "1AbCdEfGhIjKlMnOp", Name: name, Database: database, Schedule: "nightly"}
}

var restoreDay = time.Date(2026, 10, 26, 3, 0, 0, 0, time.UTC)

func TestRestoreSpecDefaultsToANewDatabase(t *testing.T) {
	spec, err := restoreSpec("nightly", backupFile("postgres/app", "nightly-postgres-app-20261026T030000Z.dump"),
		RestoreRequest{Mode: RestoreNew}, allEngines, restoreDay)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Target != "app_restored_20261026" || spec.Source != "app" || spec.Engine != EnginePostgres ||
		spec.Suffix != ".dump" {
		t.Errorf("unexpected spec %+v", spec)
	}
}

func TestRestoreSpecRefusals(t *testing.T) {
	pg := backupFile("postgres/app", "nightly-postgres-app-20261026T030000Z.dump")
	tests := []struct {
		name    string
		file    File
		request RestoreRequest
		message string
	}{
		// A schedule's page restores only what that schedule made.
		{"another schedule's file", File{ID: pg.ID, Name: pg.Name, Database: pg.Database, Schedule: "other"},
			RestoreRequest{Mode: RestoreNew}, "not one of"},
		{"the roles file", backupFile("postgres/@globals", "nightly-postgres-@globals-x.sql.gz"),
			RestoreRequest{Mode: RestoreNew}, "roles file"},
		{"an untagged file", File{ID: pg.ID, Name: "x.dump", Schedule: "nightly"},
			RestoreRequest{Mode: RestoreNew}, "not a database backup"},
		{"not an archive", backupFile("postgres/app", "notes.txt"), RestoreRequest{Mode: RestoreNew}, "not an archive"},
		// Replace destroys the live database, so it needs the name typed back.
		{"replace unconfirmed", pg, RestoreRequest{Mode: RestoreReplace}, "confirm"},
		{"replace confirmed wrongly", pg, RestoreRequest{Mode: RestoreReplace, Confirm: "App"}, "confirm"},
		// "new" into the source would be a replace without the confirmation.
		{"new into the source", pg, RestoreRequest{Mode: RestoreNew, Target: "app"}, "other than"},
		{"a target name with a quote", pg, RestoreRequest{Mode: RestoreNew, Target: "x'; drop"}, "not a database name"},
		{"no mode", pg, RestoreRequest{}, "new database, or replace"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := restoreSpec("nightly", test.file, test.request, allEngines, restoreDay)
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), test.message) {
				t.Errorf("expected an invalid request about %q, got %v", test.message, err)
			}
		})
	}
}

func TestReplaceRestoresOverTheSource(t *testing.T) {
	spec, err := restoreSpec("nightly", backupFile("mongo/events", "nightly-mongo-events-x.archive.gz"),
		RestoreRequest{Mode: RestoreReplace, Confirm: "events", Target: "ignored"}, allEngines, restoreDay)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Target != "events" || spec.Suffix != ".archive.gz" {
		t.Errorf("replace must restore over the source, got %+v", spec)
	}
}

// The file is read from Drive by id; nothing about it is taken from the request.
func TestRestoreReadsTheFileFromDrive(t *testing.T) {
	schedules := newFakeSchedules()
	schedules.schedules["nightly"] = validSchedule()
	google := &fakeGoogle{files: []File{backupFile("postgres/app", "nightly-postgres-app-x.dump")}}
	service := NewService(Dependencies{
		Google: google, Grants: &fakeGrants{grant: &Grant{RefreshToken: "r"}}, Schedules: schedules,
		Config: testGoogleConfig, Engines: allEngines, Logger: discard(),
	})

	if _, err := service.Restore(context.Background(), "nightly", "1AbCdEfGhIjKlMnOp",
		RestoreRequest{Mode: RestoreNew, Target: "app_copy"}, "op"); err != nil {
		t.Fatal(err)
	}
	if len(schedules.restored) != 1 || schedules.restored[0].Source != "app" ||
		schedules.restored[0].Target != "app_copy" {
		t.Errorf("unexpected restore %+v", schedules.restored)
	}

	if _, err := service.Restore(context.Background(), "nightly", "../../etc",
		RestoreRequest{Mode: RestoreNew}, "op"); !errors.Is(err, ErrInvalid) {
		t.Errorf("a file id that is not one must be refused before Drive, got %v", err)
	}
}

func TestARestoreJobTakesNothingAboutThePodFromTheRequest(t *testing.T) {
	config := testScheduleConfig()
	cronJob := buildCronJob(validSchedule(), "", config)
	cronJob.UID = "cron-uid"
	spec := RestoreSpec{FileID: "1AbCdEfGhIjKlMnOp", Engine: EnginePostgres, Source: "app", Target: "app_copy",
		Mode: RestoreNew, Suffix: ".dump"}

	job := buildRestoreJob(cronJob, spec, "op", config)
	pod := job.Spec.Template.Spec

	if pod.ServiceAccountName != config.ServiceAccount || job.Namespace != config.Namespace {
		t.Error("account and namespace must come from config")
	}
	if len(pod.InitContainers) != 1 || pod.InitContainers[0].Image != config.UploadImage ||
		pod.InitContainers[0].Args[0] != DownloadCommand {
		t.Errorf("the first step must download with the API's own image, got %+v", pod.InitContainers)
	}
	if len(pod.Containers) != 1 || pod.Containers[0].Image != config.PostgresImage ||
		strings.Join(pod.Containers[0].Args, " ") != "new app app_copy /work/restore.dump" {
		t.Errorf("unexpected restore container %+v", pod.Containers)
	}
	if job.OwnerReferences[0].UID != "cron-uid" || job.Labels[labelOperation] != operationRestore {
		t.Error("a restore must belong to its schedule and say it is a restore")
	}
	for _, env := range append(pod.InitContainers[0].Env, pod.Containers[0].Env...) {
		if strings.HasPrefix(env.Name, "ADMIN_") && env.ValueFrom == nil {
			t.Errorf("%s must be a secretKeyRef", env.Name)
		}
	}

	read := runFrom(job, "")
	if read.Operation != operationRestore || read.Target != "postgres/app_copy" || !read.Manual {
		t.Errorf("a restore run must read back as one, got %+v", read)
	}
}
