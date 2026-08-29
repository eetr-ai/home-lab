package helm

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

// migrationLockID names the advisory lock the migration runner takes.
//
// A constant rather than hashtext() of a name, because the value is what
// identifies the lock and it must not change when somebody renames something.
// Arbitrary, and only has to be unique among whatever else takes session-level
// advisory locks on this server — nothing else in this lab does.
const migrationLockID int64 = 7_940_331_105_220_611

// migrate brings the schema up to date, and is safe to run from every replica at
// once.
//
// The advisory lock is what makes that true: two replicas starting together would
// otherwise both see an empty schema_migrations and both try to create the same
// tables, and one of them would fail on a duplicate. The lock is held for the
// whole run and released by closing the connection, so a replica killed
// mid-migration does not leave it held.
//
// Each file runs in its own transaction and is recorded in the same transaction,
// so a migration is either applied and recorded or neither. There is no `down`
// direction: reversing a schema change on a live database is a decision somebody
// makes with the data in front of them, not something a runner should offer.
func migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	files, err := migrationFiles()
	if err != nil {
		return err
	}

	connection, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("take a connection to migrate on: %w", err)
	}
	defer connection.Release()

	if _, err := connection.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("take the migration lock: %w", err)
	}
	defer func() {
		if _, err := connection.Exec(ctx, "SELECT pg_advisory_unlock($1)", migrationLockID); err != nil {
			logger.Warn("could not release the migration lock", slog.Any("error", err))
		}
	}()

	if err := ensureMigrationTable(ctx, connection.Conn()); err != nil {
		return err
	}

	applied, err := appliedMigrations(ctx, connection.Conn())
	if err != nil {
		return err
	}

	for _, name := range files {
		if applied[name] {
			continue
		}
		if err := applyMigration(ctx, connection.Conn(), name); err != nil {
			return err
		}
		logger.Info("applied a schema migration", slog.String("migration", name))
	}
	return nil
}

// migrationFiles lists the embedded migrations in the order they must run.
func migrationFiles() ([]string, error) {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read the embedded migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	// Filename order is the running order, which is why they are numbered.
	sort.Strings(names)
	return names, nil
}

func ensureMigrationTable(ctx context.Context, conn *pgx.Conn) error {
	const statement = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name       text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`
	if _, err := conn.Exec(ctx, statement); err != nil {
		return fmt.Errorf("create the migration table: %w", err)
	}
	return nil
}

func appliedMigrations(ctx context.Context, conn *pgx.Conn) (map[string]bool, error) {
	rows, err := conn.Query(ctx, "SELECT name FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("read the applied migrations: %w", err)
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("read an applied migration: %w", err)
		}
		applied[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the applied migrations: %w", err)
	}
	return applied, nil
}

func applyMigration(ctx context.Context, conn *pgx.Conn, name string) error {
	statements, err := migrations.ReadFile("migrations/" + name)
	if err != nil {
		return fmt.Errorf("read the migration %s: %w", name, err)
	}

	transaction, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin the migration %s: %w", name, err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	if _, err := transaction.Exec(ctx, string(statements)); err != nil {
		return fmt.Errorf("apply the migration %s: %w", name, err)
	}
	if _, err := transaction.Exec(ctx,
		"INSERT INTO schema_migrations (name) VALUES ($1)", name); err != nil {
		return fmt.Errorf("record the migration %s: %w", name, err)
	}

	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit the migration %s: %w", name, err)
	}
	return nil
}
