package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// UploadCommand is the subcommand a backup pod's upload container runs.
//
// Here rather than in main because UploadSpec.Args builds a command line with it
// and main dispatches on it, and the two must agree.
const UploadCommand = "backup-upload"

// The file names the dump scripts leave in the scratch volume, by suffix. The
// scripts' header comments are the other half of this contract.
var dumpSuffixes = []string{".dump", ".sql.gz", ".archive.gz"}

const (
	failedSuffix = ".failed"
	// fileSeparator splits engine from database: postgres__app.dump. Two
	// underscores because neither engine name contains them, so the first pair is
	// always the boundary even when a database name has one.
	fileSeparator = "__"
)

// UploadSpec is one run of backup-upload, in the form it survives a process
// boundary.
type UploadSpec struct {
	Schedule string
	FolderID string
	Keep     int
	Dir      string
}

// Args renders the spec as the command line the upload container runs.
func (s UploadSpec) Args() []string {
	return []string{UploadCommand,
		"--schedule", s.Schedule,
		"--folder", s.FolderID,
		"--keep", strconv.Itoa(s.Keep),
		"--dir", s.Dir,
	}
}

// parseUploadArgs reads back what Args wrote, refusing anything it did not.
func parseUploadArgs(args []string) (UploadSpec, error) {
	var ret UploadSpec
	if len(args)%2 != 0 {
		return UploadSpec{}, fmt.Errorf("%w: every argument needs a value", ErrInvalid)
	}
	for i := 0; i < len(args); i += 2 {
		value := args[i+1]
		switch args[i] {
		case "--schedule":
			ret.Schedule = value
		case "--folder":
			ret.FolderID = value
		case "--dir":
			ret.Dir = value
		case "--keep":
			keep, err := strconv.Atoi(value)
			if err != nil {
				return UploadSpec{}, fmt.Errorf("%w: --keep must be a number, and is %q", ErrInvalid, value)
			}
			ret.Keep = keep
		default:
			return UploadSpec{}, fmt.Errorf("%w: %q is not a known argument", ErrInvalid, args[i])
		}
	}

	return ret, ret.validate()
}

func (s UploadSpec) validate() error {
	if !scheduleName.MatchString(s.Schedule) || !folderID.MatchString(s.FolderID) || s.Keep < 1 || s.Dir == "" {
		return fmt.Errorf("%w: --schedule, --folder, --keep and --dir are all required", ErrInvalid)
	}
	return nil
}

// Files is the part of Drive a run needs.
type Files interface {
	Upload(ctx context.Context, folderID, name, schedule, database string,
		body io.Reader, size int64) (File, error)
	List(ctx context.Context, folderID, schedule string) ([]File, error)
	Trash(ctx context.Context, fileID string) error
}

// RunUpload is the backup-upload command: it reads its credentials from the
// environment the CronJob gave it, and hands the real work to uploadRun.
//
// The exit code is the answer. It becomes the pod's and then the Job's, which is
// what the panel's run list shows — so anything that failed, a dump, an upload, a
// trash, comes back as an error here even when everything else worked.
func RunUpload(ctx context.Context, args []string, logger *slog.Logger) error {
	spec, err := parseUploadArgs(args)
	if err != nil {
		return err
	}

	clientID := os.Getenv("ADMIN_GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("ADMIN_GOOGLE_CLIENT_SECRET")
	refreshToken := os.Getenv("ADMIN_GOOGLE_REFRESH_TOKEN")
	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("%w: the Google OAuth client Secret is missing", ErrNotConfigured)
	}
	if refreshToken == "" {
		return fmt.Errorf("%w: connect a Google account on the backups page, then run this again",
			ErrNotConnected)
	}

	files := NewGoogleClient(clientID, clientSecret).Drive(ctx, refreshToken)
	return uploadRun(ctx, files, spec, time.Now(), logger)
}

// uploadRun uploads what the dumps left, then trashes what is past the count.
//
// In that order, and pruning only the databases whose upload just succeeded: see
// toTrash for why a failure must never cost history.
func uploadRun(ctx context.Context, files Files, spec UploadSpec, now time.Time, logger *slog.Logger) error {
	dumps, failed, err := readWork(spec.Dir)
	if err != nil {
		return err
	}
	for _, database := range failed {
		logger.Error("this database was not dumped; see the dump container's log above",
			slog.String("database", database))
	}

	stamp := now.UTC().Format("20060102T150405Z")
	succeeded := map[string]bool{}
	var problems []error

	for _, dump := range dumps {
		name := fmt.Sprintf("%s-%s-%s%s", spec.Schedule, strings.ReplaceAll(dump.database, "/", "-"),
			stamp, dump.suffix)
		logger.Info("Uploading a backup...", slog.String("file", name))
		if err := uploadOne(ctx, files, spec, name, dump); err != nil {
			problems = append(problems, err)
			continue
		}
		succeeded[dump.database] = true
	}

	problems = append(problems, prune(ctx, files, spec, succeeded, logger)...)

	if len(failed) > 0 {
		problems = append(problems, fmt.Errorf("%d database(s) could not be dumped: %s",
			len(failed), strings.Join(failed, ", ")))
	}
	if len(problems) > 0 {
		return errors.Join(problems...)
	}
	logger.Info("Finished the backup.", slog.Int("uploaded", len(succeeded)))
	return nil
}

func uploadOne(ctx context.Context, files Files, spec UploadSpec, name string, dump workFile) error {
	file, err := os.Open(dump.path)
	if err != nil {
		return fmt.Errorf("open %s: %w", dump.path, err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("read the size of %s: %w", dump.path, err)
	}
	if _, err := files.Upload(ctx, spec.FolderID, name, spec.Schedule, dump.database, file, info.Size()); err != nil {
		return err
	}
	return nil
}

func prune(ctx context.Context, files Files, spec UploadSpec, succeeded map[string]bool,
	logger *slog.Logger,
) []error {
	if len(succeeded) == 0 {
		return nil
	}
	existing, err := files.List(ctx, spec.FolderID, spec.Schedule)
	if err != nil {
		// Nothing is trashed when the listing fails: the backups are all there,
		// just one more than asked for, until the next run.
		return []error{err}
	}

	var ret []error
	for _, file := range toTrash(existing, spec.Keep, succeeded) {
		logger.Info("Moving an old backup to the trash...", slog.String("file", file.Name))
		if err := files.Trash(ctx, file.ID); err != nil {
			ret = append(ret, err)
		}
	}
	return ret
}

// workFile is one dump the scripts left behind.
type workFile struct {
	path     string
	database string
	suffix   string
}

// readWork sorts the scratch volume into dumps and failures.
//
// Anything it does not recognise is ignored rather than uploaded: the volume is
// also the tools' home directory, and a stray dotfile is not a backup.
func readWork(dir string) (dumps []workFile, failed []string, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read the dumps in %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if database, ok := databaseOf(name, failedSuffix); ok {
			failed = append(failed, database)
			continue
		}
		for _, suffix := range dumpSuffixes {
			if database, ok := databaseOf(name, suffix); ok {
				dumps = append(dumps, workFile{path: filepath.Join(dir, name), database: database, suffix: suffix})
				break
			}
		}
	}
	sort.Strings(failed)
	return dumps, failed, nil
}

// databaseOf reads postgres__app.dump as postgres/app.
func databaseOf(name, suffix string) (string, bool) {
	stem, found := strings.CutSuffix(name, suffix)
	if !found {
		return "", false
	}
	engine, database, found := strings.Cut(stem, fileSeparator)
	if !found || (engine != EnginePostgres && engine != EngineMongo) || database == "" {
		return "", false
	}
	return engine + "/" + database, true
}
