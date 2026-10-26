package backup

import (
	_ "embed"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The dump scripts, run by the init containers. Embedded rather than shipped in a
// ConfigMap so the pod spec stays a pure function of this binary and its
// configuration: there is no second object a schedule depends on that could be
// edited, deleted or out of date.
var (
	//go:embed scripts/dump-postgres.sh
	dumpPostgresScript string
	//go:embed scripts/dump-mongo.sh
	dumpMongoScript string
)

// The labels and annotations a schedule's objects carry.
//
// Identity is labelSchedule: the CronJob's name is backup-<schedule>, but its
// Jobs and pods are named by Kubernetes, and every lookup goes by the label.
const (
	labelSchedule = "home-lab.example/backup-schedule"

	// The request as it was accepted, for reading it back. The args are what the
	// pod acts on; these are what the panel shows, written from the same request
	// in the same function, so they cannot say different things.
	annotationDatabases  = "home-lab.example/backup-databases"
	annotationFolderID   = "home-lab.example/backup-folder-id"
	annotationFolderName = "home-lab.example/backup-folder-name"
	annotationKeep       = "home-lab.example/backup-keep"
	annotationActor      = "home-lab.example/backup-actor"

	// annotationManual is what kubectl create job --from writes, so a run started
	// from the panel and one started from a terminal read the same.
	annotationManual = "cronjob.kubernetes.io/instantiate"

	// scheduleComponent is what these objects are, and the value of labelName.
	scheduleComponent = "admin-backup"
	namePrefix        = "backup-"
	workDir           = "/work"
	workVolume        = "work"
)

// nonRootUser is distroless' nonroot, which every pod the panel creates runs as.
// The postgres and mongo images run their tools happily as any uid.
const nonRootUser int64 = 65532

// startingDeadline is how late a missed run may still start: a controller that
// was down at 03:00 and back at 03:40 still takes that night's backup.
const startingDeadline int64 = 3600

// historyLimit is how many finished runs of each kind the CronJob keeps around,
// which is what the panel's run list shows.
const historyLimit int32 = 5

// ScheduleConfig is everything about a backup pod that does not come from the
// request.
//
// Which is all of it, the same as the Helm slice's JobConfig: the images, the
// ServiceAccount, the namespace and every credential reference are read from this
// process's own environment. A caller influences only the validated fields of a
// ScheduleRequest — see buildCronJob, and the test that says so.
type ScheduleConfig struct {
	Namespace       string
	ServiceAccount  string
	UploadImage     string
	PostgresImage   string
	MongoImage      string
	ImagePullPolicy corev1.PullPolicy
	PullSecrets     []string
	TTLSeconds      int32
	Deadline        time.Duration
	WorkSize        resource.Quantity
	Resources       corev1.ResourceRequirements

	// Where each credential lives. Names and keys, never values: a CronJob is not
	// a Secret, and anything able to list them here could read a literal.
	Postgres     SecretRef
	Mongo        SecretRef
	ClientID     SecretRef
	ClientSecret SecretRef
	RefreshToken SecretRef
}

// SecretRef locates one value in one Secret.
type SecretRef struct {
	Name string
	Key  string
}

// configured reports whether there is a Secret to point at.
func (r SecretRef) configured() bool {
	return r.Name != "" && r.Key != ""
}

// Engines reports which servers this lab has a credential for, which is which
// ones a schedule may name.
func (c ScheduleConfig) Engines() map[string]bool {
	return map[string]bool{EnginePostgres: c.Postgres.configured(), EngineMongo: c.Mongo.configured()}
}

// buildCronJob renders a schedule as a CronJob.
//
// Nothing in the pod spec comes off the wire. The ServiceAccount, the images, the
// namespace, the commands and every credential are this process's configuration;
// the request contributes the cron line, the zone, the database names, the folder,
// and the count — each validated before it gets here, and none of them able to
// change what runs or as whom. That matters because the API may create a CronJob
// in this namespace at all, and a request able to name an image or an account
// would be a request able to run anything as anyone here.
//
// managed-by is admin-api, never Helm: see the labels in types.go.
func buildCronJob(request ScheduleRequest, actor string, config ScheduleConfig) *batchv1.CronJob {
	labels := scheduleLabels(request.Name)

	databases, _ := json.Marshal(request.Databases)
	annotations := map[string]string{
		annotationDatabases:  string(databases),
		annotationFolderID:   request.Folder.ID,
		annotationFolderName: request.Folder.Name,
		annotationKeep:       strconv.Itoa(request.Keep),
	}
	if actor != "" {
		annotations[annotationActor] = actor
	}

	var zone *string
	if request.TimeZone != "" {
		zone = &request.TimeZone
	}
	suspend := request.Suspended
	deadline := startingDeadline
	history := historyLimit

	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:        namePrefix + request.Name,
			Namespace:   config.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: request.Cron,
			TimeZone: zone,
			Suspend:  &suspend,
			// Forbid: two dumps of one database racing to upload, and then both
			// pruning, would each count the other's file and keep one fewer.
			ConcurrencyPolicy:          batchv1.ForbidConcurrent,
			StartingDeadlineSeconds:    &deadline,
			SuccessfulJobsHistoryLimit: &history,
			FailedJobsHistoryLimit:     &history,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       jobSpec(request, config),
			},
		},
	}
}

func scheduleLabels(name string) map[string]string {
	return map[string]string{
		labelName:      scheduleComponent,
		labelManagedBy: managedBy,
		labelSchedule:  name,
	}
}

func jobSpec(request ScheduleRequest, config ScheduleConfig) batchv1.JobSpec {
	// Never retried. A run that failed has already said why in its log, and a
	// second attempt minutes later would mostly fail the same way while uploading
	// whatever the first one managed twice.
	backoffLimit := int32(0)
	deadline := int64(config.Deadline.Seconds())

	return batchv1.JobSpec{
		BackoffLimit:            &backoffLimit,
		ActiveDeadlineSeconds:   &deadline,
		TTLSecondsAfterFinished: &config.TTLSeconds,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: scheduleLabels(request.Name)},
			Spec:       podSpec(request, config),
		},
	}
}

func podSpec(request ScheduleRequest, config ScheduleConfig) corev1.PodSpec {
	nonRoot := true
	// The backup pod never talks to the Kubernetes API, so it holds no token for
	// it. Its ServiceAccount exists only so it is not the namespace's default.
	automount := false

	ret := corev1.PodSpec{
		RestartPolicy:                corev1.RestartPolicyNever,
		ServiceAccountName:           config.ServiceAccount,
		AutomountServiceAccountToken: &automount,
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot:   &nonRoot,
			SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
		InitContainers: dumpContainers(request, config),
		Containers:     []corev1.Container{uploadContainer(request, config)},
		Volumes: []corev1.Volume{{
			Name: workVolume,
			VolumeSource: corev1.VolumeSource{
				// Bounded, because it is on the node's disk and an unbounded dump is
				// somebody else's outage. Exceeding it evicts the pod, which reads as
				// a failed run with the reason in its events.
				EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: &config.WorkSize},
			},
		}},
	}
	for _, secret := range config.PullSecrets {
		ret.ImagePullSecrets = append(ret.ImagePullSecrets, corev1.LocalObjectReference{Name: secret})
	}
	return ret
}

// dumpContainers is one init container per engine the schedule names.
//
// Init containers rather than sidecars because the order is the point: every
// dump is finished, and failed or not recorded, before the upload starts.
func dumpContainers(request ScheduleRequest, config ScheduleConfig) []corev1.Container {
	var ret []corev1.Container

	if names := namesFor(request.Databases, EnginePostgres); len(names) > 0 {
		ret = append(ret, container("dump-postgres", config.PostgresImage, config,
			[]string{"sh", "-c", dumpPostgresScript, "dump-postgres"}, names,
			secretEnv("ADMIN_POSTGRES_DSN", config.Postgres, false)))
	}
	if names := namesFor(request.Databases, EngineMongo); len(names) > 0 {
		ret = append(ret, container("dump-mongo", config.MongoImage, config,
			[]string{"sh", "-c", dumpMongoScript, "dump-mongo"}, names,
			secretEnv("ADMIN_MONGO_URI", config.Mongo, false)))
	}
	return ret
}

// uploadContainer runs this same binary's backup-upload command.
//
// The Google credentials are optional references. A lab that disconnected Drive
// still has its schedules, and a required reference to a missing Secret leaves a
// pod stuck in CreateContainerConfigError until the deadline — whereas an empty
// variable lets backup-upload fail at once and say that no account is connected.
func uploadContainer(request ScheduleRequest, config ScheduleConfig) corev1.Container {
	args := UploadSpec{
		Schedule: request.Name,
		FolderID: request.Folder.ID,
		Keep:     request.Keep,
		Dir:      workDir,
	}.Args()

	// No command: the image's entrypoint is the admin-api binary.
	return container("upload", config.UploadImage, config, nil, args,
		secretEnv("ADMIN_GOOGLE_CLIENT_ID", config.ClientID, true),
		secretEnv("ADMIN_GOOGLE_CLIENT_SECRET", config.ClientSecret, true),
		secretEnv("ADMIN_GOOGLE_REFRESH_TOKEN", config.RefreshToken, true),
	)
}

func container(name, image string, config ScheduleConfig, command, args []string,
	env ...corev1.EnvVar,
) corev1.Container {
	nonRoot := true
	noEscalation := false
	readOnlyRoot := true
	user := nonRootUser

	return corev1.Container{
		Name:            name,
		Image:           image,
		ImagePullPolicy: config.ImagePullPolicy,
		Command:         command,
		Args:            args,
		Env: append([]corev1.EnvVar{
			{Name: "WORK", Value: workDir},
			// Some tools reach for a home directory, and the root filesystem is
			// read-only, so home is the scratch volume.
			{Name: "HOME", Value: workDir},
		}, env...),
		Resources:    config.Resources,
		VolumeMounts: []corev1.VolumeMount{{Name: workVolume, MountPath: workDir}},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &noEscalation,
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			ReadOnlyRootFilesystem:   &readOnlyRoot,
			RunAsNonRoot:             &nonRoot,
			RunAsUser:                &user,
			RunAsGroup:               &user,
		},
	}
}

func secretEnv(name string, ref SecretRef, optional bool) corev1.EnvVar {
	ret := corev1.EnvVar{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: ref.Name},
				Key:                  ref.Key,
			},
		},
	}
	if optional {
		ret.ValueFrom.SecretKeyRef.Optional = &optional
	}
	return ret
}

// manualJob builds a run of a schedule right now, the way `kubectl create job
// --from=cronjob/...` does: the CronJob's own template, owned by the CronJob so
// deleting the schedule takes it along.
func manualJob(cronJob *batchv1.CronJob, actor string) *batchv1.Job {
	isController := true
	annotations := map[string]string{annotationManual: "manual"}
	if actor != "" {
		annotations[annotationActor] = actor
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: cronJob.Name + "-run-",
			Namespace:    cronJob.Namespace,
			Labels:       cronJob.Spec.JobTemplate.Labels,
			Annotations:  annotations,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "batch/v1",
				Kind:       "CronJob",
				Name:       cronJob.Name,
				UID:        cronJob.UID,
				Controller: &isController,
			}},
		},
		Spec: *cronJob.Spec.JobTemplate.Spec.DeepCopy(),
	}
}

// scheduleFrom reads a CronJob back into a schedule.
//
// From the annotations buildCronJob wrote. One that cannot be read is reported
// as empty rather than failing the listing: a schedule somebody edited by hand
// should still appear, so they can see it and fix or delete it.
func scheduleFrom(cronJob *batchv1.CronJob) Schedule {
	ret := Schedule{
		ScheduleRequest: ScheduleRequest{
			Name: cronJob.Labels[labelSchedule],
			Cron: cronJob.Spec.Schedule,
			Folder: Folder{
				ID:   cronJob.Annotations[annotationFolderID],
				Name: cronJob.Annotations[annotationFolderName],
			},
		},
		CreatedAt: cronJob.CreationTimestamp.Time,
		Running:   len(cronJob.Status.Active),
		Actor:     cronJob.Annotations[annotationActor],
	}
	if ret.Name == "" {
		ret.Name = strings.TrimPrefix(cronJob.Name, namePrefix)
	}
	if cronJob.Spec.TimeZone != nil {
		ret.TimeZone = *cronJob.Spec.TimeZone
	}
	if cronJob.Spec.Suspend != nil {
		ret.Suspended = *cronJob.Spec.Suspend
	}
	ret.Keep, _ = strconv.Atoi(cronJob.Annotations[annotationKeep])
	_ = json.Unmarshal([]byte(cronJob.Annotations[annotationDatabases]), &ret.Databases)
	// An annotation that would not read leaves the list nil, which encodes as
	// null; the panel walks this list, so it gets an empty one instead.
	if ret.Databases == nil {
		ret.Databases = []DatabaseRef{}
	}

	if cronJob.Status.LastScheduleTime != nil {
		ret.LastScheduled = &cronJob.Status.LastScheduleTime.Time
	}
	if cronJob.Status.LastSuccessfulTime != nil {
		ret.LastSucceeded = &cronJob.Status.LastSuccessfulTime.Time
	}
	return ret
}

// runFrom reads a Job into a run.
func runFrom(job *batchv1.Job, pod string) Run {
	phase, reason := phaseOf(job.Status)
	ret := Run{
		Name:      job.Name,
		Phase:     phase,
		Reason:    reason,
		Manual:    job.Annotations[annotationManual] != "",
		Pod:       pod,
		Namespace: job.Namespace,
		CreatedAt: job.CreationTimestamp.Time,
	}
	// In the order they run, so the log picker reads top to bottom as the run did.
	template := job.Spec.Template.Spec
	for _, container := range append(append([]corev1.Container{}, template.InitContainers...),
		template.Containers...) {
		ret.Containers = append(ret.Containers, container.Name)
	}
	if job.Status.StartTime != nil {
		ret.StartedAt = &job.Status.StartTime.Time
	}
	if job.Status.CompletionTime != nil {
		ret.FinishedAt = &job.Status.CompletionTime.Time
	}
	return ret
}

// phaseOf reads a Job's status as one word, and why.
//
// The terminal conditions are checked before the active count: a Job that has
// just failed can briefly report both, and "running" for one that already lost is
// the wrong answer to give. This is the Helm slice's rule, restated rather than
// imported, because a slice never imports another's internals.
func phaseOf(status batchv1.JobStatus) (phase, reason string) {
	for _, condition := range status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		switch condition.Type {
		// FailureTarget and SuccessCriteriaMet are the verdict arriving before
		// the pods have finished terminating; Failed and Complete follow them.
		case batchv1.JobComplete, batchv1.JobSuccessCriteriaMet:
			return PhaseSucceeded, ""
		case batchv1.JobFailed, batchv1.JobFailureTarget:
			return PhaseFailed, condition.Reason
		}
	}
	if status.Active > 0 {
		return PhaseRunning, ""
	}
	return PhasePending, ""
}
