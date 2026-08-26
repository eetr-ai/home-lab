package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	// maxQueryRows bounds a result. A SELECT with no LIMIT over a large table
	// would otherwise be sent to a browser in full.
	maxQueryRows = 200
	// queryTimeout bounds how long the server will run one statement. Set as
	// statement_timeout so PostgreSQL cancels the query itself rather than the
	// client abandoning a query that keeps running.
	queryTimeout = 15 * time.Second
	// queryDeadline bounds the whole operation from the client's side: connecting,
	// executing, and reading the rows back. Longer than queryTimeout so a query
	// the server kills reports the server's own message — which says what it
	// refused — rather than a context deadline that says nothing.
	queryDeadline = 25 * time.Second
)

// Query runs one read-only statement against a database and returns its rows.
//
// It runs over a *separate connection*, authenticated as a role that is not a
// superuser. That is the boundary, and it is the only thing here that is one.
//
// The obvious cheaper design — keep the superuser pool and drop privileges for
// the transaction with SET LOCAL ROLE — does not work, and it is worth writing
// down why, because it looks like it does. SET ROLE is reversible by whoever
// authenticated: verified against PostgreSQL 18, a submitted `RESET ROLE` (or
// `SET ROLE NONE`, or `DO $$ BEGIN RESET ROLE; END $$`) restored superuser
// inside the read-only transaction, after which
// `COPY (SELECT 1) TO PROGRAM 'id > /tmp/escaped'` ran and left a file owned by
// the postgres user on the database host. session_user is what SET ROLE resets
// to, so nothing done inside a session can lower that floor.
//
// With a genuinely non-superuser login, all four of those escapes leave
// current_user unchanged, and pg_read_file and COPY TO PROGRAM are both refused
// as permission errors. Also verified rather than assumed.
//
// The READ ONLY transaction and the statement timeout remain, as the second
// layer: the query role should hold no write privileges, but a role gains grants
// over time and this does not depend on that staying true. Neither is the
// boundary on its own — a read-only transaction refuses writes and DDL and does
// not refuse COPY TO PROGRAM.
//
// No pattern matching over the SQL text anywhere. Comments, CTEs, dollar quoting
// and DO blocks all defeat one — the DO block above is the demonstration — and a
// single miss would be the whole boundary.
func (r *Repository) Query(ctx context.Context, database, sql string) (QueryResult, error) {
	if r.queryConfig == nil {
		return QueryResult{}, ErrQueryUnavailable
	}

	// One deadline over the whole operation, before anything opens a socket.
	// statement_timeout is set after connecting and bounds only execution, so on
	// its own it leaves both ends unbounded: a connection to an unresponsive
	// server, and the delivery of rows that pgx reads inside collectRows. Neither
	// is execution, so neither is what the server-side timeout covers.
	ctx, cancel := context.WithTimeout(ctx, queryDeadline)
	defer cancel()

	conn, err := r.connectAsQueryRole(ctx, database)
	if err != nil {
		return QueryResult{}, err
	}
	defer func() { _ = conn.Close(ctx) }()

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return QueryResult{}, fmt.Errorf("begin a read-only transaction: %w", err)
	}
	// Always. The success path does not commit — nothing in it should have
	// changed anything, and rolling back means a statement that somehow did
	// leaves nothing behind.
	defer func() { _ = tx.Rollback(ctx) }()

	// SET LOCAL, so it lasts exactly as long as this transaction rather than
	// leaking onto a pooled connection.
	if _, err := tx.Exec(ctx,
		fmt.Sprintf("SET LOCAL statement_timeout = %d", queryTimeout.Milliseconds())); err != nil {
		return QueryResult{}, fmt.Errorf("bound the query: %w", err)
	}

	started := time.Now()
	// One statement per message, because pgx uses the extended protocol: verified
	// that "SELECT 1; DROP TABLE t" is refused rather than run as two.
	rows, err := tx.Query(ctx, sql)
	if err != nil {
		// The server's own message, which is the useful part: a syntax error names
		// the position, and a refusal names what it would not run.
		return QueryResult{}, fmt.Errorf("%w: %s", ErrQueryFailed, err.Error())
	}
	defer rows.Close()

	result, err := collectRows(rows)
	if err != nil {
		return QueryResult{}, err
	}
	result.ElapsedMs = time.Since(started).Milliseconds()
	return result, nil
}

// connectAsQueryRole opens a connection to one database as the query credential.
func (r *Repository) connectAsQueryRole(ctx context.Context, database string) (*pgx.Conn, error) {
	if _, err := quoteIdentifier(database); err != nil {
		return nil, err
	}

	config := r.queryConfig.ConnConfig.Copy()
	config.Database = database

	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("connect to database %q for a query: %w", database, err)
	}
	if err := verifyStringsAreConforming(ctx, conn); err != nil {
		_ = conn.Close(ctx)
		return nil, err
	}
	return conn, nil
}

// collectRows reads a result set into strings, stopping at the row cap.
func collectRows(rows pgx.Rows) (QueryResult, error) {
	result := QueryResult{Columns: []string{}, Rows: [][]string{}}
	for _, field := range rows.FieldDescriptions() {
		result.Columns = append(result.Columns, field.Name)
	}

	for rows.Next() {
		if len(result.Rows) == maxQueryRows {
			// Stop reading rather than reading and discarding: the connection is
			// closed straight after, which cancels the rest at the server.
			result.Truncated = true
			break
		}

		values, err := rows.Values()
		if err != nil {
			return QueryResult{}, fmt.Errorf("%w: %s", ErrQueryFailed, err.Error())
		}
		result.Rows = append(result.Rows, renderRow(values))
	}

	if err := rows.Err(); err != nil && !result.Truncated {
		// Not when truncated: abandoning a result set mid-read is how the cap
		// works, and pgx reports that as an error on the rows.
		return QueryResult{}, fmt.Errorf("%w: %s", ErrQueryFailed, err.Error())
	}
	return result, nil
}

// renderRow turns one row's values into strings.
//
// Everything becomes text because the panel displays it and computes nothing on
// it. Carrying every PostgreSQL type faithfully through JSON would mean a type
// map to keep current — and the browser would render most of it as a string
// regardless.
func renderRow(values []any) []string {
	rendered := make([]string, 0, len(values))
	for _, value := range values {
		if value == nil {
			// Distinguishable from the empty string, which is a different value and
			// looks identical once both are rendered as text.
			rendered = append(rendered, "NULL")
			continue
		}
		rendered = append(rendered, fmt.Sprintf("%v", value))
	}
	return rendered
}
