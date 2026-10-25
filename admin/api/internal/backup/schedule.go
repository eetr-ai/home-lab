package backup

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	// The API image is distroless and carries no zoneinfo, so a schedule's time
	// zone could not be checked without this. It costs the binary about 450KB.
	_ "time/tzdata"
)

// The engines a schedule can dump.
const (
	EnginePostgres = "postgres"
	EngineMongo    = "mongo"
)

// globalsDatabase is the name the PostgreSQL roles and tablespaces are filed
// under. It cannot collide with a real database because databaseName refuses @.
const globalsDatabase = "@globals"

// Bounds on what a schedule may ask for.
const (
	// maxScheduleName leaves room for the prefix and the suffixes Kubernetes
	// appends: a CronJob name may be at most 52 characters, and every name here is
	// backup-<name>.
	maxScheduleName = 45
	maxKeep         = 100
	maxDatabases    = 50
	maxFolderName   = 200
)

var (
	// scheduleName is a DNS label, because it becomes part of one.
	scheduleName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	// databaseName is an allowlist, not an escaper. Both engines accept far more
	// than this, and a name outside it is one this lab does not use; what it buys
	// is that a database name is safe as a shell argument, a file name, and a
	// Drive query value without any of those layers having to think about it.
	databaseName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]{0,62}$`)
	// folderID is the shape Drive uses for every file and folder id.
	folderID = regexp.MustCompile(`^[A-Za-z0-9_-]{10,200}$`)
)

// DatabaseRef names one database on one of the lab's servers.
type DatabaseRef struct {
	// Engine is postgres or mongo.
	Engine string `json:"engine"`
	Name   string `json:"name"`
}

// key is how a database is filed in Drive and grouped for retention.
func (d DatabaseRef) key() string {
	return d.Engine + "/" + d.Name
}

// Folder is the Drive folder a schedule uploads into, as the Picker returned it.
type Folder struct {
	ID string `json:"id"`
	// Name is for the panel to show. Drive is asked by ID, so a renamed folder
	// keeps receiving backups and only this label goes stale.
	Name string `json:"name"`
}

// ScheduleRequest is what an operator sends to create or replace a schedule.
type ScheduleRequest struct {
	// Name is required on create and ignored on replace, where the path names it.
	Name string `json:"name,omitempty"`
	// Cron is a standard five-field cron expression, or a macro such as @daily.
	// The API server is what checks it; see Service.CreateSchedule.
	Cron string `json:"cron"`
	// TimeZone is an IANA zone such as America/Los_Angeles. Empty is the
	// controller's own, which on most clusters is UTC.
	TimeZone  string        `json:"timeZone,omitempty"`
	Databases []DatabaseRef `json:"databases"`
	Folder    Folder        `json:"folder"`
	// Keep is how many backups of each database to keep in the folder.
	Keep      int  `json:"keep"`
	Suspended bool `json:"suspended"`
}

// Schedule is one scheduled backup, as its CronJob describes it.
type Schedule struct {
	ScheduleRequest
	CreatedAt time.Time `json:"createdAt"`
	// LastScheduled and LastSucceeded come from the CronJob's status. The gap
	// between them is the first thing to look at when backups stop arriving.
	LastScheduled *time.Time `json:"lastScheduled,omitempty"`
	LastSucceeded *time.Time `json:"lastSucceeded,omitempty"`
	// Running counts the runs in progress, which concurrencyPolicy keeps at one.
	Running int    `json:"running"`
	Actor   string `json:"actor,omitempty"`
}

// Run is one execution of a schedule.
type Run struct {
	Name string `json:"name"`
	// Phase is pending, running, succeeded, or failed.
	Phase  string `json:"phase"`
	Reason string `json:"reason,omitempty"`
	// Manual says somebody pressed "run now", rather than the clock.
	Manual bool `json:"manual"`
	// Pod is empty until one has been scheduled. Its log is where a run explains
	// itself: which database failed to dump, which upload Google refused.
	Pod        string     `json:"pod,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// File is one backup a schedule left in its folder.
type File struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Database string `json:"database"`
	// Size is in bytes. Drive reports it as a string; it is parsed on the way in.
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"createdAt"`
	WebViewLink string    `json:"webViewLink,omitempty"`
}

// The phases a run can be in.
const (
	PhasePending   = "pending"
	PhaseRunning   = "running"
	PhaseSucceeded = "succeeded"
	PhaseFailed    = "failed"
)

// validate checks a schedule before anything reaches the cluster.
//
// engines says which servers this lab manages. A schedule naming a database on a
// server the panel has no credential for would create a CronJob whose pods can
// never start, so it is refused here instead.
func (r ScheduleRequest) validate(engines map[string]bool) error {
	if len(r.Name) > maxScheduleName || !scheduleName.MatchString(r.Name) {
		return fmt.Errorf("%w: the name must be a lowercase DNS label of at most %d characters",
			ErrInvalid, maxScheduleName)
	}
	if strings.TrimSpace(r.Cron) == "" {
		return fmt.Errorf("%w: a cron schedule is required", ErrInvalid)
	}
	if r.TimeZone != "" {
		if _, err := time.LoadLocation(r.TimeZone); err != nil || r.TimeZone == "Local" {
			return fmt.Errorf("%w: %q is not a time zone; use an IANA name such as UTC", ErrInvalid, r.TimeZone)
		}
	}
	if r.Keep < 1 || r.Keep > maxKeep {
		return fmt.Errorf("%w: keep between 1 and %d backups of each database", ErrInvalid, maxKeep)
	}
	if !folderID.MatchString(r.Folder.ID) {
		return fmt.Errorf("%w: choose a Drive folder", ErrInvalid)
	}
	if len(r.Folder.Name) > maxFolderName {
		return fmt.Errorf("%w: the folder name is longer than %d characters", ErrInvalid, maxFolderName)
	}
	return validateDatabases(r.Databases, engines)
}

func validateDatabases(databases []DatabaseRef, engines map[string]bool) error {
	if len(databases) == 0 || len(databases) > maxDatabases {
		return fmt.Errorf("%w: choose between 1 and %d databases", ErrInvalid, maxDatabases)
	}

	seen := map[string]bool{}
	for _, database := range databases {
		if database.Engine != EnginePostgres && database.Engine != EngineMongo {
			return fmt.Errorf("%w: %q is not an engine; use postgres or mongo", ErrInvalid, database.Engine)
		}
		if !engines[database.Engine] {
			return fmt.Errorf("%w: this lab does not manage %s, so it cannot back it up",
				ErrInvalid, database.Engine)
		}
		if !databaseName.MatchString(database.Name) {
			return fmt.Errorf("%w: %q is not a database name this panel backs up", ErrInvalid, database.Name)
		}
		if seen[database.key()] {
			return fmt.Errorf("%w: %s is listed twice", ErrInvalid, database.key())
		}
		seen[database.key()] = true
	}
	return nil
}

// namesFor lists the databases of one engine, in the order they were given.
func namesFor(databases []DatabaseRef, engine string) []string {
	var ret []string
	for _, database := range databases {
		if database.Engine == engine {
			ret = append(ret, database.Name)
		}
	}
	return ret
}
