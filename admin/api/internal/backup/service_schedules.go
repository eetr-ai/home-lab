package backup

import (
	"context"
	"fmt"
)

// ScheduleStore keeps the schedules, which are CronJobs.
type ScheduleStore interface {
	ListSchedules(ctx context.Context) ([]Schedule, error)
	ReadSchedule(ctx context.Context, name string) (Schedule, error)
	CreateSchedule(ctx context.Context, request ScheduleRequest, actor string) error
	UpdateSchedule(ctx context.Context, request ScheduleRequest, actor string) error
	DeleteSchedule(ctx context.Context, name string) error
	RunSchedule(ctx context.Context, name, actor string) (Run, error)
	ListRuns(ctx context.Context, name string) ([]Run, error)
}

// ListSchedules returns every schedule.
func (s *Service) ListSchedules(ctx context.Context) ([]Schedule, error) {
	return s.schedules.ListSchedules(ctx)
}

// ReadSchedule returns one schedule.
func (s *Service) ReadSchedule(ctx context.Context, name string) (Schedule, error) {
	if err := validateName(name); err != nil {
		return Schedule{}, err
	}
	return s.schedules.ReadSchedule(ctx, name)
}

// CreateSchedule declares a new scheduled backup.
//
// The cron line is not parsed here. The API server parses it, with the same code
// that will run it, and its refusal comes back as a 400 naming the field — a
// second parser here could only ever disagree with the one that matters.
//
// Creating one needs an OAuth client but not a connected account: a schedule set
// up before connecting simply fails its runs, loudly, until somebody connects.
func (s *Service) CreateSchedule(ctx context.Context, request ScheduleRequest, actor string) (Schedule, error) {
	if s.google == nil {
		return Schedule{}, ErrNotConfigured
	}
	if err := request.validate(s.engines); err != nil {
		return Schedule{}, err
	}
	if err := s.schedules.CreateSchedule(ctx, request, actor); err != nil {
		return Schedule{}, err
	}
	return s.schedules.ReadSchedule(ctx, request.Name)
}

// UpdateSchedule replaces a schedule. The path names it; a name in the body is
// ignored, because renaming a CronJob is not a thing Kubernetes does.
func (s *Service) UpdateSchedule(ctx context.Context, name string, request ScheduleRequest,
	actor string,
) (Schedule, error) {
	request.Name = name
	if err := request.validate(s.engines); err != nil {
		return Schedule{}, err
	}
	if err := s.schedules.UpdateSchedule(ctx, request, actor); err != nil {
		return Schedule{}, err
	}
	return s.schedules.ReadSchedule(ctx, name)
}

// DeleteSchedule removes a schedule and its runs. Its backups stay in Drive.
func (s *Service) DeleteSchedule(ctx context.Context, name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	return s.schedules.DeleteSchedule(ctx, name)
}

// RunSchedule starts a run now, whatever the clock says.
func (s *Service) RunSchedule(ctx context.Context, name, actor string) (Run, error) {
	if err := validateName(name); err != nil {
		return Run{}, err
	}
	return s.schedules.RunSchedule(ctx, name, actor)
}

// ListRuns returns a schedule's recent runs.
func (s *Service) ListRuns(ctx context.Context, name string) ([]Run, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	return s.schedules.ListRuns(ctx, name)
}

// ListFiles returns the backups a schedule has kept, read from the folder itself.
func (s *Service) ListFiles(ctx context.Context, name string) ([]File, error) {
	if s.google == nil {
		return nil, ErrNotConfigured
	}
	schedule, err := s.ReadSchedule(ctx, name)
	if err != nil {
		return nil, err
	}
	grant, err := s.grants.ReadGrant(ctx)
	if err != nil {
		return nil, err
	}
	files, err := s.google.ListBackups(ctx, grant.RefreshToken, schedule.Folder.ID, name)
	if err != nil {
		return nil, fmt.Errorf("list the backups of %s: %w", name, err)
	}
	return files, nil
}

func validateName(name string) error {
	if len(name) > maxScheduleName || !scheduleName.MatchString(name) {
		return fmt.Errorf("%w: %q is not a schedule name", ErrInvalid, name)
	}
	return nil
}
