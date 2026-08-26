package postgres

import (
	"context"
	"fmt"
	"strconv"
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
// It runs as the same account the rest of this slice connects as — the panel's
// administrative credential — over a short-lived connection to the named
// database. There is no second credential: the panel is given a superuser so it
// can administer the server, and the console is one more thing it does with it.
//
// What bounds a statement here is therefore not authority but shape:
//
//   - a READ ONLY transaction that is always rolled back, so every INSERT,
//     UPDATE, DELETE and DDL is refused by the server;
//   - statement_timeout, so a runaway query is killed by the server rather than
//     abandoned by the client;
//   - pgx's extended protocol, asked for by name rather than inherited, so one
//     statement goes per message and `SELECT 1; DROP TABLE t` is refused rather
//     than run as two.
//
// Be clear about what that does not bound. A superuser session reaches outside
// the database: `COPY (SELECT 1) TO PROGRAM ...` runs a shell command as the
// server's own user, and a READ ONLY transaction does not refuse it, because it
// is not a database write. Nor can the session lower its own floor, by either
// route: SET ROLE changes current_user and RESET ROLE puts it back, while SET
// SESSION AUTHORIZATION changes session_user as well and RESET SESSION
// AUTHORIZATION still puts that back — PostgreSQL keeps the identity the
// connection actually authenticated as, and it is a superuser. So a submitted
// statement that appears to give up privilege is undone by the next one.
// Verified against PostgreSQL 18.
//
// How far that reaches is a property of the deployment rather than of this code.
// The home lab runs PostgreSQL in an unprivileged container with one bind mount
// for its data directory (databases/postgres.compose.yaml), so it is the
// container and that directory, not root on the machine hosting it. Somewhere
// that ran the server on the host directly, it would be the host.
//
// That is the deliberate position: the caller is already authenticated as an
// operator and the same account already creates and drops databases, roles and
// users through the endpoints next to this one. If that ever stops being true —
// a panel with viewers as well as operators, or a server not in a container of
// its own — this is the endpoint to give its own non-superuser login, and the
// two verified facts above are why nothing inside the session would substitute
// for one.
//
// No pattern matching over the SQL text anywhere. Comments, CTEs, dollar quoting
// and DO blocks all defeat one, and a check here would suggest a boundary that
// the paragraph above says plainly is not there.
func (r *Repository) Query(ctx context.Context, database, sql string) (QueryResult, error) {
	// One deadline over the whole operation, before anything opens a socket.
	// statement_timeout is set after connecting and bounds only execution, so on
	// its own it leaves both ends unbounded: a connection to an unresponsive
	// server, and the delivery of rows that pgx reads inside collectRows. Neither
	// is execution, so neither is what the server-side timeout covers.
	ctx, cancel := context.WithTimeout(ctx, queryDeadline)
	defer cancel()

	conn, err := r.connectTo(ctx, database)
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

	// set_config rather than SET LOCAL, with is_local true — which does the same
	// thing, and reverts on commit exactly the same way. The reason for the longer
	// spelling is that SET is a utility statement and takes no bind parameters, so
	// it forces the value to be interpolated into the SQL; set_config is an
	// ordinary function and takes one. The value here is an int64 from a constant
	// in this file, so interpolating it was never an injection — but "safe because
	// of where this number comes from" is an argument a reader has to reconstruct,
	// and a static analyser cannot. This needs neither.
	if _, err := tx.Exec(ctx,
		"SELECT set_config('statement_timeout', $1, true)",
		strconv.FormatInt(queryTimeout.Milliseconds(), 10)); err != nil {
		return QueryResult{}, fmt.Errorf("bound the query: %w", err)
	}

	started := time.Now()
	// One statement per message, because this asks for the extended protocol
	// rather than inheriting whatever the connection defaults to: verified that
	// "SELECT 1; DROP TABLE t" is refused rather than run as two.
	//
	// Named here and not left to the default, because the default is not this
	// package's to decide. `connectTo` copies the pool's config, the pool parses
	// the DSN, and pgx reads `default_query_exec_mode` out of a connection string
	// — `simple_protocol` among the values it accepts. Under that mode the whole
	// string goes to the server as one Query message and runs as many statements,
	// so `COMMIT; INSERT ...` would end the read-only transaction above and
	// persist what followed, with the deferred Rollback left nothing to undo. A
	// bound that a deployment's DSN can switch off is not one of the three this
	// function claims to have.
	rows, err := tx.Query(ctx, sql, pgx.QueryExecModeExec)
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
