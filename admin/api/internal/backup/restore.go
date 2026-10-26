package backup

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"
)

// The ways a restore lands.
const (
	// RestoreNew restores into a database that must not exist yet, leaving the
	// live one untouched until somebody has looked.
	RestoreNew = "new"
	// RestoreReplace drops the database the backup came from and restores over it.
	RestoreReplace = "replace"
)

// RestoreRequest is what an operator asks for when restoring one backup.
type RestoreRequest struct {
	// Mode is new or replace.
	Mode string `json:"mode"`
	// Target names the new database; empty takes <database>_restored_<date>.
	// Ignored for replace, which always restores over the source.
	Target string `json:"target,omitempty"`
	// Confirm must repeat the database's name for replace. A typed name rather
	// than a checkbox, because replace is the one action here that destroys data.
	Confirm string `json:"confirm,omitempty"`
}

// RestoreSpec is a validated restore, in the form the repository builds a Job from.
type RestoreSpec struct {
	FileID string
	Engine string
	Source string
	Target string
	Mode   string
	// Suffix is the archive's file extension, which the restore tools read by.
	Suffix string
}

var (
	//go:embed scripts/restore-postgres.sh
	restorePostgresScript string
	//go:embed scripts/restore-mongo.sh
	restoreMongoScript string
)

// Restore starts a Job restoring one of a schedule's backups.
//
// Everything about the file is read from Drive, never taken from the request:
// the request names a file id, and the file's own tags say which schedule and
// database it is. A file tagged for another schedule is refused, so a schedule's
// page can only restore what that schedule made.
func (s *Service) Restore(ctx context.Context, name, fileID string, request RestoreRequest,
	actor string,
) (Run, error) {
	if s.google == nil {
		return Run{}, ErrNotConfigured
	}
	if err := validateName(name); err != nil {
		return Run{}, err
	}
	if !folderID.MatchString(fileID) {
		return Run{}, fmt.Errorf("%w: %q is not a Drive file id", ErrInvalid, fileID)
	}
	if _, err := s.schedules.ReadSchedule(ctx, name); err != nil {
		return Run{}, err
	}
	grant, err := s.grants.ReadGrant(ctx)
	if err != nil {
		return Run{}, err
	}
	file, err := s.google.ReadBackup(ctx, grant.RefreshToken, fileID)
	if err != nil {
		return Run{}, fmt.Errorf("read the backup to restore: %w", err)
	}

	spec, err := restoreSpec(name, file, request, s.engines, time.Now())
	if err != nil {
		return Run{}, err
	}
	return s.schedules.RestoreBackup(ctx, name, spec, actor)
}

// restoreSpec checks a restore against the file it names.
func restoreSpec(schedule string, file File, request RestoreRequest, engines map[string]bool,
	now time.Time,
) (RestoreSpec, error) {
	ret, err := restoreSource(schedule, file, engines)
	if err != nil {
		return RestoreSpec{}, err
	}
	ret.Mode = request.Mode
	if ret.Target, err = restoreTarget(ret.Source, request, now); err != nil {
		return RestoreSpec{}, err
	}
	return ret, nil
}

// restoreSource reads what a file is, from its own tags and name.
func restoreSource(schedule string, file File, engines map[string]bool) (RestoreSpec, error) {
	if file.Schedule != schedule {
		return RestoreSpec{}, fmt.Errorf("%w: that file is not one of %s's backups", ErrInvalid, schedule)
	}
	engine, source, found := strings.Cut(file.Database, "/")
	if !found || (engine != EnginePostgres && engine != EngineMongo) {
		return RestoreSpec{}, fmt.Errorf("%w: that file is not a database backup", ErrInvalid)
	}
	if source == globalsDatabase {
		return RestoreSpec{}, fmt.Errorf("%w: the roles file is not restored on its own; "+
			"restoring a database creates the roles it needs", ErrInvalid)
	}
	if !engines[engine] {
		return RestoreSpec{}, fmt.Errorf("%w: this lab does not manage %s", ErrInvalid, engine)
	}

	ret := RestoreSpec{FileID: file.ID, Engine: engine, Source: source}
	for _, suffix := range dumpSuffixes {
		if strings.HasSuffix(file.Name, suffix) {
			ret.Suffix = suffix
		}
	}
	if ret.Suffix == "" {
		return RestoreSpec{}, fmt.Errorf("%w: %s is not an archive this panel made", ErrInvalid, file.Name)
	}
	return ret, nil
}

// restoreTarget decides which database a restore lands in.
func restoreTarget(source string, request RestoreRequest, now time.Time) (string, error) {
	var ret string
	switch request.Mode {
	case RestoreNew:
		ret = request.Target
		if ret == "" {
			ret = fmt.Sprintf("%s_restored_%s", source, now.UTC().Format("20060102"))
		}
		if ret == source {
			return "", fmt.Errorf("%w: a new database needs a name other than %s", ErrInvalid, source)
		}
	case RestoreReplace:
		ret = source
		if request.Confirm != source {
			return "", fmt.Errorf("%w: type %s to confirm replacing it", ErrInvalid, source)
		}
	default:
		return "", fmt.Errorf("%w: restore into a new database, or replace", ErrInvalid)
	}
	if !databaseName.MatchString(ret) {
		return "", fmt.Errorf("%w: %q is not a database name this panel uses", ErrInvalid, ret)
	}
	return ret, nil
}
