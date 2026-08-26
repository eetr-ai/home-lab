package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository is the PostgreSQL-backed persistence for this slice.
//
// It holds a pool against the server's own `postgres` database for everything
// server-wide, and opens a short-lived connection to a named database for the
// operations that are per database — an extension can only be created from inside
// the database that will hold it.
type Repository struct {
	pool   *pgxpool.Pool
	config *pgxpool.Config
	// queryConfig is a separate, non-superuser credential used only by the
	// read-only query console. Nil when none is configured, which switches the
	// console off — see Query for why it cannot share the pool above.
	queryConfig *pgxpool.Config
}

// NewRepository connects to the server and verifies the assumptions this slice's
// SQL depends on.
func NewRepository(ctx context.Context, dsn string) (*Repository, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse the PostgreSQL connection string: %w", err)
	}

	// quoteLiteral doubles single quotes, which is sufficient only while
	// standard_conforming_strings is on. It has been the default since PostgreSQL
	// 9.1, but a server started with it off would turn every quoted literal in
	// this slice into an injection point, so it is checked rather than assumed.
	//
	// Checked per connection rather than once at startup, for two reasons: the
	// guarantee then holds for every connection this pool ever hands out, and
	// construction stays lazy. An API that refused to start because PostgreSQL was
	// unreachable would take the whole panel down — including the pages that would
	// have told an operator why — which is the same reason /healthz checks nothing
	// downstream.
	config.AfterConnect = verifyStringsAreConforming

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("configure the PostgreSQL pool: %w", err)
	}

	return &Repository{pool: pool, config: config}, nil
}

// WithQueryCredential attaches the connection the query console runs statements
// as. Without one, the console is not served.
func (r *Repository) WithQueryCredential(dsn string) error {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse the PostgreSQL query connection string: %w", err)
	}
	config.AfterConnect = verifyStringsAreConforming
	r.queryConfig = config
	return nil
}

// verifyStringsAreConforming refuses a connection to a server whose literal
// escaping this slice cannot rely on.
func verifyStringsAreConforming(ctx context.Context, conn *pgx.Conn) error {
	var conforming string
	if err := conn.QueryRow(ctx, "SHOW standard_conforming_strings").Scan(&conforming); err != nil {
		return fmt.Errorf("read standard_conforming_strings: %w", err)
	}
	if conforming != "on" {
		return errors.New("standard_conforming_strings is off; this server cannot be administered safely")
	}
	return nil
}

// Close releases the connection pool.
func (r *Repository) Close() {
	r.pool.Close()
}

// ListDatabases returns every database on the server.
func (r *Repository) ListDatabases(ctx context.Context) ([]Database, error) {
	const query = `
		SELECT d.datname,
		       pg_catalog.pg_get_userbyid(d.datdba),
		       pg_catalog.pg_encoding_to_char(d.encoding),
		       -- Size is reported only where it can be read. pg_database_size
		       -- raises for a database the role cannot connect to, and one
		       -- inaccessible database would otherwise fail the whole listing
		       -- rather than simply reporting an unknown size.
		       CASE WHEN d.datallowconn AND pg_catalog.has_database_privilege(d.datname, 'CONNECT')
		            THEN pg_catalog.pg_database_size(d.datname)
		            ELSE 0
		       END
		FROM pg_catalog.pg_database d
		ORDER BY d.datname`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	defer rows.Close()

	databases := []Database{}
	for rows.Next() {
		var database Database
		if err := rows.Scan(&database.Name, &database.Owner, &database.Encoding, &database.SizeBytes); err != nil {
			return nil, fmt.Errorf("read a database row: %w", err)
		}
		databases = append(databases, database)
	}
	return databases, rows.Err() //nolint:wrapcheck // rows.Err is already a pgx error with context
}

// DatabaseExists reports whether a database is present.
func (r *Repository) DatabaseExists(ctx context.Context, name string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_database WHERE datname = $1)", name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check for database %q: %w", name, err)
	}
	return exists, nil
}

// CreateDatabase creates a database, optionally owned by a role.
func (r *Repository) CreateDatabase(ctx context.Context, name, owner string) error {
	quotedName, err := quoteIdentifier(name)
	if err != nil {
		return err
	}

	statement := "CREATE DATABASE " + quotedName
	if owner != "" {
		quotedOwner, ownerErr := quoteIdentifier(owner)
		if ownerErr != nil {
			return ownerErr
		}
		statement += " OWNER " + quotedOwner
	}

	// CREATE DATABASE cannot run inside a transaction block, so this is a bare
	// Exec rather than part of one.
	if _, err := r.pool.Exec(ctx, statement); err != nil {
		return fmt.Errorf("create database %q: %w", name, err)
	}
	return nil
}

// DropDatabase removes a database and everything in it.
//
// Deliberately not `WITH (FORCE)`. Forcing terminates every other session on the
// database, and a drop that fails because something is still connected is worth
// seeing: it usually means the caller is dropping the wrong one.
func (r *Repository) DropDatabase(ctx context.Context, name string) error {
	quotedName, err := quoteIdentifier(name)
	if err != nil {
		return err
	}
	if _, err := r.pool.Exec(ctx, "DROP DATABASE "+quotedName); err != nil {
		return fmt.Errorf("drop database %q: %w", name, err)
	}
	return nil
}

// ListRoles returns every role on the server.
func (r *Repository) ListRoles(ctx context.Context) ([]Role, error) {
	const query = `
		SELECT rolname, rolcanlogin, rolsuper, rolcreatedb, rolcreaterole, rolconnlimit
		FROM pg_catalog.pg_roles
		ORDER BY rolname`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	roles := []Role{}
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.Name, &role.CanLogin, &role.IsSuperuser,
			&role.CanCreateDatabase, &role.CanCreateRole, &role.ConnectionLimit); err != nil {
			return nil, fmt.Errorf("read a role row: %w", err)
		}
		roles = append(roles, role)
	}
	return roles, rows.Err() //nolint:wrapcheck // rows.Err is already a pgx error with context
}

// RoleExists reports whether a role is present.
func (r *Repository) RoleExists(ctx context.Context, name string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)", name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check for role %q: %w", name, err)
	}
	return exists, nil
}

// CurrentRole reports the role the panel is connected as.
func (r *Repository) CurrentRole(ctx context.Context) (string, error) {
	var role string
	if err := r.pool.QueryRow(ctx, "SELECT current_user").Scan(&role); err != nil {
		return "", fmt.Errorf("read the current role: %w", err)
	}
	return role, nil
}

// CreateRole creates a role.
func (r *Repository) CreateRole(ctx context.Context, req CreateRoleRequest) error {
	statement, err := createRoleStatement(req)
	if err != nil {
		return err
	}
	// The statement carries the password as a literal, so it must never reach a
	// log. pgx does not log statements by default; nothing here adds one.
	if _, err := r.pool.Exec(ctx, statement); err != nil {
		return fmt.Errorf("create role %q: %w", req.Name, err)
	}
	return nil
}

// createRoleStatement builds the CREATE ROLE, kept separate so it can be reasoned
// about — and, being pure, tested — without a server.
func createRoleStatement(req CreateRoleRequest) (string, error) {
	quotedName, err := quoteIdentifier(req.Name)
	if err != nil {
		return "", err
	}

	statement := "CREATE ROLE " + quotedName + " WITH NOSUPERUSER"
	if req.CanCreateDatabase {
		statement += " CREATEDB"
	}
	if req.CanCreateRole {
		statement += " CREATEROLE"
	}
	if !req.CanLogin {
		return statement + " NOLOGIN", nil
	}

	// The verifier, never the password. PostgreSQL accepts a pre-computed
	// SCRAM-SHA-256 verifier in place of a password, so the plaintext never
	// reaches the server — where it would otherwise be visible in
	// pg_stat_activity and written to the log whenever log_statement is on.
	verifier, err := scramVerifier(req.Password)
	if err != nil {
		return "", err
	}
	quotedVerifier, err := quoteLiteral(verifier)
	if err != nil {
		return "", err
	}
	return statement + " LOGIN PASSWORD " + quotedVerifier, nil
}

// DropRole removes a role.
func (r *Repository) DropRole(ctx context.Context, name string) error {
	quotedName, err := quoteIdentifier(name)
	if err != nil {
		return err
	}
	if _, err := r.pool.Exec(ctx, "DROP ROLE "+quotedName); err != nil {
		return fmt.Errorf("drop role %q: %w", name, err)
	}
	return nil
}

// ListExtensions returns the extensions installed in one database.
func (r *Repository) ListExtensions(ctx context.Context, database string) ([]Extension, error) {
	conn, err := r.connectTo(ctx, database)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close(ctx) }()

	rows, err := conn.Query(ctx,
		"SELECT extname, extversion FROM pg_catalog.pg_extension ORDER BY extname")
	if err != nil {
		return nil, fmt.Errorf("list extensions in %q: %w", database, err)
	}
	defer rows.Close()

	extensions := []Extension{}
	for rows.Next() {
		var extension Extension
		if err := rows.Scan(&extension.Name, &extension.Version); err != nil {
			return nil, fmt.Errorf("read an extension row: %w", err)
		}
		extensions = append(extensions, extension)
	}
	return extensions, rows.Err() //nolint:wrapcheck // rows.Err is already a pgx error with context
}

// CreateExtension installs an extension into one database.
func (r *Repository) CreateExtension(ctx context.Context, database, name string) error {
	quotedName, err := quoteExtensionName(name)
	if err != nil {
		return err
	}

	conn, err := r.connectTo(ctx, database)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+quotedName); err != nil {
		return fmt.Errorf("create extension %q in %q: %w", name, database, err)
	}
	return nil
}

// connectTo opens a connection to one database on the same server.
//
// Extensions are per database and can only be created from inside the database
// that will hold them, so the pool — which is bound to the administrative
// database — cannot serve these. The connection is short-lived because these
// operations are rare; a pool per database would keep connections open against
// every database anyone has ever looked at.
func (r *Repository) connectTo(ctx context.Context, database string) (*pgx.Conn, error) {
	if _, err := quoteIdentifier(database); err != nil {
		return nil, err
	}

	config := r.config.ConnConfig.Copy()
	config.Database = database

	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("connect to database %q: %w", database, err)
	}
	// The pool's AfterConnect does not apply to a connection made directly.
	if err := verifyStringsAreConforming(ctx, conn); err != nil {
		_ = conn.Close(ctx)
		return nil, err
	}
	return conn, nil
}
