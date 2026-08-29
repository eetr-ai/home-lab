package helm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// uniqueViolation is PostgreSQL's code for a broken unique constraint, which is
// how "this namespace already has a release by that name" arrives.
const uniqueViolation = "23505"

// Store is the PostgreSQL record of what this lab has declared.
//
// It holds desired state only. Nothing about the cluster is written here, and
// nothing here is trusted to describe the cluster: what is running comes from
// Helm's storage on every request. That separation is what lets a release be
// changed by hand, or by another tool, without this table quietly becoming wrong
// about it — instead the panel reports drift, which is a true statement.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore connects to the database and brings its schema up to date.
//
// Construction is lazy in the same way the other database slices are: pgxpool
// does not dial here, so an unreachable server costs a failed request rather
// than a failed startup. Migrating does dial, and a failure there is returned
// rather than fatal — the caller logs it and leaves the deployment routes
// answering 503, because an API that refuses to start because PostgreSQL is down
// takes with it the pages that would have said so.
func NewStore(ctx context.Context, dsn string, logger *slog.Logger) (*Store, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse the Helm connection string: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("configure the Helm pool: %w", err)
	}

	if err := migrate(ctx, pool, logger); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

// Close releases the connection pool.
func (s *Store) Close() {
	s.pool.Close()
}

// ListDeployments returns the declared deployments, newest first. An empty
// namespace means every namespace.
func (s *Store) ListDeployments(ctx context.Context, namespace string) ([]Deployment, error) {
	const query = `
		SELECT id, namespace, release_name, chart_ref, created_by, created_at
		FROM helm_deployments
		WHERE ($1 = '' OR namespace = $1)
		ORDER BY namespace, release_name`

	rows, err := s.pool.Query(ctx, query, namespace)
	if err != nil {
		return nil, storeError("list the deployments", err)
	}
	defer rows.Close()

	var deployments []Deployment
	for rows.Next() {
		var deployment Deployment
		if err := rows.Scan(&deployment.ID, &deployment.Namespace, &deployment.ReleaseName,
			&deployment.ChartRef, &deployment.CreatedBy, &deployment.CreatedAt); err != nil {
			return nil, storeError("read a deployment", err)
		}
		deployments = append(deployments, deployment)
	}
	if err := rows.Err(); err != nil {
		return nil, storeError("list the deployments", err)
	}
	return deployments, nil
}

// ReadDeployment returns one deployment by id.
func (s *Store) ReadDeployment(ctx context.Context, id string) (Deployment, error) {
	const query = `
		SELECT id, namespace, release_name, chart_ref, created_by, created_at
		FROM helm_deployments WHERE id = $1`

	var deployment Deployment
	err := s.pool.QueryRow(ctx, query, id).Scan(&deployment.ID, &deployment.Namespace,
		&deployment.ReleaseName, &deployment.ChartRef, &deployment.CreatedBy, &deployment.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Deployment{}, fmt.Errorf("%w: no deployment %s", ErrNotFound, id)
	}
	if err != nil {
		return Deployment{}, storeError("read the deployment", err)
	}
	return deployment, nil
}

// CreateDeployment records a new deployment and its first version, together.
//
// One transaction, because a deployment with no versions is a row the rest of
// this slice has no answer for: every read projects the newest version, and
// "there isn't one" would have to be special-cased everywhere rather than made
// impossible here.
func (s *Store) CreateDeployment(ctx context.Context, deployment Deployment,
	first DeploymentVersion,
) (Deployment, error) {
	deployment.ID = uuid.NewString()

	transaction, err := s.pool.Begin(ctx)
	if err != nil {
		return Deployment{}, storeError("begin recording the deployment", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	const insertDeployment = `
		INSERT INTO helm_deployments (id, namespace, release_name, chart_ref, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at`
	err = transaction.QueryRow(ctx, insertDeployment, deployment.ID, deployment.Namespace,
		deployment.ReleaseName, deployment.ChartRef, deployment.CreatedBy).Scan(&deployment.CreatedAt)
	if isUniqueViolation(err) {
		return Deployment{}, fmt.Errorf("%w: %s already has a deployment called %s",
			ErrAlreadyExists, deployment.Namespace, deployment.ReleaseName)
	}
	if err != nil {
		return Deployment{}, storeError("record the deployment", err)
	}

	if _, err := insertVersion(ctx, transaction, deployment.ID, 1, first); err != nil {
		return Deployment{}, err
	}

	if err := transaction.Commit(ctx); err != nil {
		return Deployment{}, storeError("record the deployment", err)
	}
	return deployment, nil
}

// DeleteDeployment forgets a deployment and every version of it.
//
// The release on the cluster is untouched. Forgetting the record and removing
// the workload are different intentions, and doing both because somebody asked
// for one would be the kind of surprise there is no undo for.
func (s *Store) DeleteDeployment(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, "DELETE FROM helm_deployments WHERE id = $1", id)
	if err != nil {
		return storeError("forget the deployment", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: no deployment %s", ErrNotFound, id)
	}
	return nil
}

// ListVersions returns every declared version, newest first.
func (s *Store) ListVersions(ctx context.Context, id string) ([]DeploymentVersion, error) {
	const query = `
		SELECT version, chart_version, values_yaml, source, created_by, created_at,
		       rolled_out_at, helm_revision
		FROM helm_deployment_versions
		WHERE deployment_id = $1
		ORDER BY version DESC`

	rows, err := s.pool.Query(ctx, query, id)
	if err != nil {
		return nil, storeError("list the versions", err)
	}
	defer rows.Close()

	var versions []DeploymentVersion
	for rows.Next() {
		var version DeploymentVersion
		if err := rows.Scan(&version.Version, &version.ChartVersion, &version.ValuesYAML,
			&version.Source, &version.CreatedBy, &version.CreatedAt,
			&version.RolledOutAt, &version.HelmRevision); err != nil {
			return nil, storeError("read a version", err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, storeError("list the versions", err)
	}
	return versions, nil
}

// AppendVersion adds the next version of a deployment.
//
// The number is worked out inside the statement rather than read, incremented
// and written back, so two writers racing produce two versions rather than one
// overwriting the other. The primary key is what settles it if they collide.
func (s *Store) AppendVersion(ctx context.Context, id string,
	version DeploymentVersion,
) (DeploymentVersion, error) {
	transaction, err := s.pool.Begin(ctx)
	if err != nil {
		return DeploymentVersion{}, storeError("begin adding the version", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	// Locked so the number this reads is still the highest when it writes.
	var next int
	err = transaction.QueryRow(ctx, `
		SELECT coalesce(max(version), 0) + 1
		FROM helm_deployment_versions
		WHERE deployment_id = $1
		FOR UPDATE`, id).Scan(&next)
	if err != nil {
		return DeploymentVersion{}, storeError("work out the next version", err)
	}

	stored, err := insertVersion(ctx, transaction, id, next, version)
	if err != nil {
		return DeploymentVersion{}, err
	}
	if err := transaction.Commit(ctx); err != nil {
		return DeploymentVersion{}, storeError("add the version", err)
	}
	return stored, nil
}

// ReadVersion returns one declared version.
func (s *Store) ReadVersion(ctx context.Context, id string, number int) (DeploymentVersion, error) {
	const query = `
		SELECT version, chart_version, values_yaml, source, created_by, created_at,
		       rolled_out_at, helm_revision
		FROM helm_deployment_versions
		WHERE deployment_id = $1 AND version = $2`

	var version DeploymentVersion
	err := s.pool.QueryRow(ctx, query, id, number).Scan(&version.Version, &version.ChartVersion,
		&version.ValuesYAML, &version.Source, &version.CreatedBy, &version.CreatedAt,
		&version.RolledOutAt, &version.HelmRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeploymentVersion{}, fmt.Errorf("%w: no version %d", ErrNotFound, number)
	}
	if err != nil {
		return DeploymentVersion{}, storeError("read the version", err)
	}
	return version, nil
}

// MarkRolledOut stamps a version with when it reached the cluster.
//
// Called from the detached job after Helm reports success. A failure leaves the
// stamp null, which is what the panel reads as "declared, never applied" — the
// reason it failed lives on the release, where Helm wrote it.
func (s *Store) MarkRolledOut(ctx context.Context, id string, number, helmRevision int) error {
	const statement = `
		UPDATE helm_deployment_versions
		SET rolled_out_at = $3, helm_revision = $4
		WHERE deployment_id = $1 AND version = $2`

	if _, err := s.pool.Exec(ctx, statement, id, number, time.Now().UTC(), helmRevision); err != nil {
		return storeError("record the rollout", err)
	}
	return nil
}

func insertVersion(ctx context.Context, transaction pgx.Tx, id string, number int,
	version DeploymentVersion,
) (DeploymentVersion, error) {
	const statement = `
		INSERT INTO helm_deployment_versions
			(deployment_id, version, chart_version, values_yaml, source, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING version, created_at`

	err := transaction.QueryRow(ctx, statement, id, number, version.ChartVersion,
		version.ValuesYAML, version.Source, version.CreatedBy).
		Scan(&version.Version, &version.CreatedAt)
	if err != nil {
		return DeploymentVersion{}, storeError("add the version", err)
	}
	return version, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

// storeError wraps a driver failure as something the handler can turn into a 503.
//
// Every database failure reads the same way to a caller — the record could not be
// reached — and the distinction that matters is between "the record says no" and
// "the record could not be asked". Only the second is a 503.
func storeError(doing string, err error) error {
	return fmt.Errorf("%w: could not %s: %w", ErrStoreUnavailable, doing, err)
}
