package backup

import (
	"reflect"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testScheduleConfig() ScheduleConfig {
	return ScheduleConfig{
		Namespace:      "admin",
		ServiceAccount: "admin-backup",
		UploadImage:    "registry.example.invalid/admin-api:1.0.0",
		PostgresImage:  "postgres:18.1-bookworm",
		MongoImage:     "mongo:7.0.40-jammy",
		TTLSeconds:     86400,
		Deadline:       2 * time.Hour,
		WorkSize:       resource.MustParse("10Gi"),
		Postgres:       SecretRef{Name: "admin-postgres", Key: "dsn"},
		Mongo:          SecretRef{Name: "admin-mongo", Key: "uri"},
		ClientID:       SecretRef{Name: "admin-google-drive", Key: "clientId"},
		ClientSecret:   SecretRef{Name: "admin-google-drive", Key: "clientSecret"},
		RefreshToken:   SecretRef{Name: "admin-google-drive-token", Key: refreshTokenKey},
	}
}

func allContainers(job *batchv1.CronJob) []corev1.Container {
	pod := job.Spec.JobTemplate.Spec.Template.Spec
	return append(append([]corev1.Container{}, pod.InitContainers...), pod.Containers...)
}

// The API may create CronJobs here, and Kubernetes does not check whether the
// creator may run as the pod's account. So the guarantee is in this function: the
// account, the images, the namespace and the commands are the configuration's,
// whatever the request says.
func TestBuildCronJobTakesNothingAboutThePodFromTheRequest(t *testing.T) {
	config := testScheduleConfig()
	request := validSchedule()
	job := buildCronJob(request, "op@example.invalid", config)
	pod := job.Spec.JobTemplate.Spec.Template.Spec

	if job.Namespace != config.Namespace || pod.ServiceAccountName != config.ServiceAccount {
		t.Errorf("namespace and account must come from config, got %q, %q", job.Namespace, pod.ServiceAccountName)
	}
	images := map[string]string{}
	for _, container := range allContainers(job) {
		images[container.Name] = container.Image
	}
	want := map[string]string{
		"dump-postgres": config.PostgresImage, "dump-mongo": config.MongoImage, "upload": config.UploadImage,
	}
	if !reflect.DeepEqual(images, want) {
		t.Errorf("images must come from config: got %v", images)
	}
	if pod.AutomountServiceAccountToken == nil || *pod.AutomountServiceAccountToken {
		t.Error("the backup pod never talks to the Kubernetes API and must hold no token for it")
	}
}

// A CronJob is not a Secret: anything that can list them can read a literal in
// one. So every credential is a reference. ScheduleConfig holds no values to
// leak in the first place, which is the deeper half of this guarantee.
func TestBuildCronJobCarriesCredentialsAsReferencesOnly(t *testing.T) {
	job := buildCronJob(validSchedule(), "", testScheduleConfig())

	for _, container := range allContainers(job) {
		for _, env := range container.Env {
			sensitive := strings.Contains(env.Name, "DSN") || strings.Contains(env.Name, "URI") ||
				strings.Contains(env.Name, "GOOGLE")
			if sensitive && (env.Value != "" || env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil) {
				t.Errorf("%s in %s must be a secretKeyRef and nothing else", env.Name, container.Name)
			}
		}
	}
}

// Optional only where a missing Secret should be reported by the uploader rather
// than leave the pod stuck: the Google credentials, which disconnecting deletes.
// A database credential the chart rendered is never optional.
func TestOnlyTheGoogleCredentialsAreOptional(t *testing.T) {
	job := buildCronJob(validSchedule(), "", testScheduleConfig())
	for _, container := range allContainers(job) {
		for _, env := range container.Env {
			if env.ValueFrom == nil {
				continue
			}
			optional := env.ValueFrom.SecretKeyRef.Optional != nil && *env.ValueFrom.SecretKeyRef.Optional
			if optional != strings.HasPrefix(env.Name, "ADMIN_GOOGLE_") {
				t.Errorf("%s: optional=%v", env.Name, optional)
			}
		}
	}
}

func TestOnlyTheEnginesAScheduleNamesAreDumped(t *testing.T) {
	request := validSchedule()
	request.Databases = []DatabaseRef{{Engine: EngineMongo, Name: "events"}, {Engine: EngineMongo, Name: "audit"}}
	pod := buildCronJob(request, "", testScheduleConfig()).Spec.JobTemplate.Spec.Template.Spec

	if len(pod.InitContainers) != 1 || pod.InitContainers[0].Name != "dump-mongo" {
		t.Fatalf("expected only the mongo dump, got %d init containers", len(pod.InitContainers))
	}
	// sh -c <script> <$0> then the names as $@, in the order given.
	if got := pod.InitContainers[0].Args; !reflect.DeepEqual(got, []string{"events", "audit"}) {
		t.Errorf("the database names should be the script's arguments, got %v", got)
	}
	if pod.InitContainers[0].Command[3] != "dump-mongo" {
		t.Error("the script's $0 should name it, so its errors say where they came from")
	}
}

func TestTheUploadArgumentsRoundTrip(t *testing.T) {
	spec := UploadSpec{Schedule: "nightly", FolderID: "1AbCdEfGhIjKlMnOp", Keep: 7, Dir: workDir}
	args := spec.Args()
	if args[0] != UploadCommand {
		t.Fatalf("the command should lead, got %v", args)
	}
	parsed, err := parseUploadArgs(args[1:])
	if err != nil || parsed != spec {
		t.Errorf("round trip lost something: %+v, %v", parsed, err)
	}

	for _, bad := range [][]string{
		{"--schedule", "nightly", "--folder", "1AbCdEfGhIjKlMnOp", "--keep", "7"},
		{"--schedule", "nightly", "--folder", "1AbCdEfGhIjKlMnOp", "--keep", "x", "--dir", "/w"},
		{"--schedule", "nightly", "--folder", "1AbCdEfGhIjKlMnOp", "--keep", "7", "--dir", "/w", "--extra", "1"},
		{"--schedule"},
	} {
		if _, err := parseUploadArgs(bad); err == nil {
			t.Errorf("%v should be refused", bad)
		}
	}
}

func TestAScheduleReadsBackAsItWasWritten(t *testing.T) {
	request := validSchedule()
	request.Suspended = true
	job := buildCronJob(request, "op@example.invalid", testScheduleConfig())
	job.CreationTimestamp = metav1.Now()

	got := scheduleFrom(job)
	if !reflect.DeepEqual(got.ScheduleRequest, request) || got.Actor != "op@example.invalid" {
		t.Errorf("read back %+v, wrote %+v", got.ScheduleRequest, request)
	}
}

// Forbid is what stops two runs both pruning and each keeping one fewer.
func TestRunsNeverOverlapAndNeverRetry(t *testing.T) {
	job := buildCronJob(validSchedule(), "", testScheduleConfig())
	if job.Spec.ConcurrencyPolicy != batchv1.ForbidConcurrent {
		t.Error("runs of one schedule must not overlap")
	}
	if *job.Spec.JobTemplate.Spec.BackoffLimit != 0 {
		t.Error("a failed run is not retried")
	}
	if job.Labels[labelManagedBy] != managedBy {
		t.Error("managed-by must be admin-api so Helm never adopts a schedule")
	}
}

func TestAManualRunIsOwnedByItsSchedule(t *testing.T) {
	cronJob := buildCronJob(validSchedule(), "", testScheduleConfig())
	cronJob.UID = "cron-uid"
	run := manualJob(cronJob, "op@example.invalid")

	if len(run.OwnerReferences) != 1 || run.OwnerReferences[0].UID != "cron-uid" {
		t.Error("deleting the schedule should take its manual runs with it")
	}
	if run.Labels[labelSchedule] != "nightly" || run.Annotations[annotationManual] == "" {
		t.Errorf("a manual run must be findable as the schedule's, and say it was manual: %v", run.Labels)
	}
	if !reflect.DeepEqual(run.Spec.Template.Spec, cronJob.Spec.JobTemplate.Spec.Template.Spec) {
		t.Error("a manual run must run exactly what the schedule runs")
	}
}
