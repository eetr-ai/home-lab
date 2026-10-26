package backup

import (
	"context"
	"fmt"
	"os"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The labels and annotations a restore Job carries on top of its schedule's.
const (
	// labelOperation says a run restores rather than backs up. A label, so the
	// schedule's runs list keeps both kinds together and still tells them apart.
	labelOperation   = "home-lab.example/backup-operation"
	operationRestore = "restore"

	annotationRestoreTarget = "home-lab.example/backup-restore-target"
	annotationRestoreMode   = "home-lab.example/backup-restore-mode"
)

// DownloadCommand is the subcommand a restore's first container runs.
const DownloadCommand = "backup-download"

// RestoreBackup starts a restore Job for one of a schedule's backups.
//
// Refused while any run of the schedule is going, backup or restore, for the
// reason RunSchedule gives and one more: a backup dumping a database halfway
// through being restored would keep a copy of neither.
func (r *Repository) RestoreBackup(ctx context.Context, name string, spec RestoreSpec, actor string) (Run, error) {
	cronJob, err := r.readCronJob(ctx, name)
	if err != nil {
		return Run{}, err
	}
	runs, err := r.ListRuns(ctx, name)
	if err != nil {
		return Run{}, err
	}
	if active := activeRun(runs); active != "" {
		return Run{}, fmt.Errorf("%w: %s is still going", ErrInProgress, active)
	}
	created, err := r.clientset.BatchV1().Jobs(r.namespace).
		Create(ctx, buildRestoreJob(cronJob, spec, actor, r.schedules), metav1.CreateOptions{})
	if err != nil {
		return Run{}, translate(err, "start restoring "+spec.Source)
	}
	return runFrom(created, ""), nil
}

// buildRestoreJob renders a restore as a Job owned by its schedule.
//
// The same rule as buildCronJob: nothing about the pod comes from the request.
// The images, the account and every credential are configuration; the request
// contributes a file id and two database names, both checked before they get here.
func buildRestoreJob(cronJob *batchv1.CronJob, spec RestoreSpec, actor string, config ScheduleConfig) *batchv1.Job {
	labels := scheduleLabels(cronJob.Labels[labelSchedule])
	labels[labelOperation] = operationRestore
	annotations := map[string]string{
		annotationRestoreTarget: spec.Engine + "/" + spec.Target,
		annotationRestoreMode:   spec.Mode,
	}
	if actor != "" {
		annotations[annotationActor] = actor
	}

	archive := workDir + "/restore" + spec.Suffix
	download := container("download", config.UploadImage, config, nil,
		[]string{DownloadCommand, "--file", spec.FileID, "--to", archive},
		secretEnv("ADMIN_GOOGLE_CLIENT_ID", config.ClientID, true),
		secretEnv("ADMIN_GOOGLE_CLIENT_SECRET", config.ClientSecret, true),
		secretEnv("ADMIN_GOOGLE_REFRESH_TOKEN", config.RefreshToken, true),
	)

	args := []string{spec.Mode, spec.Source, spec.Target, archive}
	var restore corev1.Container
	if spec.Engine == EnginePostgres {
		restore = container("restore-postgres", config.PostgresImage, config,
			[]string{"sh", "-c", restorePostgresScript, "restore-postgres"}, args,
			secretEnv("ADMIN_POSTGRES_DSN", config.Postgres, false))
	} else {
		restore = container("restore-mongo", config.MongoImage, config,
			[]string{"sh", "-c", restoreMongoScript, "restore-mongo"}, args,
			secretEnv("ADMIN_MONGO_URI", config.Mongo, false))
	}

	pod := podSpec(ScheduleRequest{}, config)
	pod.InitContainers = []corev1.Container{download}
	pod.Containers = []corev1.Container{restore}

	backoffLimit := int32(0)
	deadline := int64(config.Deadline.Seconds())
	isController := true
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: cronJob.Name + "-restore-",
			Namespace:    cronJob.Namespace,
			Labels:       labels,
			Annotations:  annotations,
			// Owned by the schedule, so deleting it takes its restores along.
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "batch/v1", Kind: "CronJob", Name: cronJob.Name, UID: cronJob.UID,
				Controller: &isController,
			}},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			ActiveDeadlineSeconds:   &deadline,
			TTLSecondsAfterFinished: &config.TTLSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       pod,
			},
		},
	}
}

// RunDownload is the backup-download command: it fetches one Drive file to a
// path, with the credentials the Job gave it.
func RunDownload(ctx context.Context, args []string) error {
	if len(args) != 4 || args[0] != "--file" || args[2] != "--to" ||
		!folderID.MatchString(args[1]) || !strings.HasPrefix(args[3], workDir+"/") {
		return fmt.Errorf("%w: usage: %s --file <id> --to %s/<name>", ErrInvalid, DownloadCommand, workDir)
	}
	clientID, clientSecret := os.Getenv("ADMIN_GOOGLE_CLIENT_ID"), os.Getenv("ADMIN_GOOGLE_CLIENT_SECRET")
	refreshToken := os.Getenv("ADMIN_GOOGLE_REFRESH_TOKEN")
	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("%w: the Google OAuth client Secret is missing", ErrNotConfigured)
	}
	if refreshToken == "" {
		return fmt.Errorf("%w: connect a Google account on the backups page, then run this again", ErrNotConnected)
	}

	out, err := os.Create(args[3])
	if err != nil {
		return fmt.Errorf("create %s: %w", args[3], err)
	}
	defer func() { _ = out.Close() }()

	written, err := NewGoogleClient(clientID, clientSecret).Drive(ctx, refreshToken).Download(ctx, args[1], out)
	if err != nil {
		return err
	}
	fmt.Printf("Downloaded %d bytes to %s.\n", written, args[3])
	if err := out.Close(); err != nil {
		return fmt.Errorf("finish writing %s: %w", args[3], err)
	}
	return nil
}
