package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// browsePageSize is how many rows one browse page holds. The same cap the query
// console uses, so a page and a `SELECT * LIMIT 200` show the same amount.
const browsePageSize = maxQueryRows

// pkColumn is one column of a primary key, with the type its cursor value is cast
// back to. Both come from the catalog, never from the caller.
type pkColumn struct {
	Name string
	Type string
}

// Browse returns one keyset-paginated page of a table or view.
//
// The page is stable across inserts and deletes because it orders by the primary
// key and continues with `(key) > (cursor)` rather than an OFFSET: a row added or
// removed before the current position cannot shift a later page onto rows already
// seen or skip rows never seen, which is exactly what OFFSET does. A relation with
// no primary key — every view, and the rare keyless table — has no such order to
// page over, so it returns a single capped page instead.
func (r *Repository) Browse(ctx context.Context, database, schema, table, cursor string) (BrowseResult, error) {
	var result BrowseResult
	err := r.inReadOnlyTx(ctx, database, func(ctx context.Context, tx pgx.Tx) error {
		if err := ensureRelationExists(ctx, tx, schema, table); err != nil {
			return err
		}
		pk, err := primaryKeyColumns(ctx, tx, schema, table)
		if err != nil {
			return err
		}

		var cursorValues []string
		if cursor != "" {
			cursorValues, err = decodeCursor(cursor)
			if err != nil {
				return err
			}
			if len(cursorValues) != len(pk) {
				// The cursor was minted for a different key shape than the one the
				// table has now — a stale link after the table changed, most likely.
				return fmt.Errorf("%w: the cursor does not match the table's primary key", ErrInvalidName)
			}
		}

		execSQL, displaySQL, err := buildBrowseSQL(schema, table, pk, len(cursorValues) > 0)
		if err != nil {
			return err
		}

		page, err := browsePage(ctx, tx, execSQL, cursorValues, len(pk))
		if err != nil {
			return err
		}
		page.SQL = displaySQL
		result = page
		return nil
	})
	return result, err
}

// browsePage runs one page's statement and shapes the result.
//
// The statement fetches one row more than a page holds, so the presence of that
// extra row is how a next page is detected without a second count query. The
// primary-key columns ride along at the end of each row as text (the ::text casts
// in buildBrowseSQL), and those become the next cursor — kept out of the visible
// columns, which are only the relation's own.
func browsePage(ctx context.Context, tx pgx.Tx, sql string, cursorValues []string, pkLen int) (BrowseResult, error) {
	// The cursor values are bound as parameters after the exec-mode marker; the
	// statement casts each back to its key column's type. See query.go for why the
	// mode is named rather than inherited.
	args := make([]any, 0, len(cursorValues)+1)
	args = append(args, pgx.QueryExecModeExec)
	for _, value := range cursorValues {
		args = append(args, value)
	}

	started := time.Now()
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return BrowseResult{}, fmt.Errorf("%w: %s", ErrQueryFailed, err.Error())
	}
	defer rows.Close()

	result := BrowseResult{Columns: []string{}, Rows: [][]string{}}
	fields := rows.FieldDescriptions()
	// The visible columns are the relation's; the trailing pkLen are the cursor
	// carriers appended by buildBrowseSQL and are never shown.
	visible := len(fields) - pkLen
	for i := 0; i < visible; i++ {
		result.Columns = append(result.Columns, fields[i].Name)
	}

	var lastCursor []string
	extra := false
	for rows.Next() {
		if len(result.Rows) == browsePageSize {
			// The one row past the page. Its existence is the whole reason it was
			// fetched: it says there is a next page. It is not shown, and reading no
			// further cancels the rest at the server when the statement is closed.
			extra = true
			break
		}
		values, err := rows.Values()
		if err != nil {
			return BrowseResult{}, fmt.Errorf("%w: %s", ErrQueryFailed, err.Error())
		}
		result.Rows = append(result.Rows, renderRow(values[:visible]))
		if pkLen > 0 {
			lastCursor = cursorFromRow(values[visible:])
		}
	}
	if err := rows.Err(); err != nil && !extra {
		return BrowseResult{}, fmt.Errorf("%w: %s", ErrQueryFailed, err.Error())
	}
	result.ElapsedMs = time.Since(started).Milliseconds()

	if extra {
		if pkLen > 0 {
			// A next page exists and there is a key to continue from.
			result.NextCursor = encodeCursor(lastCursor)
		} else {
			// A next page exists but nothing to page over, so it cannot be reached —
			// the same shape of answer as the query console's row cap.
			result.Truncated = true
		}
	}
	return result, nil
}

// cursorFromRow reads the trailing key columns of a row into cursor strings. They
// were selected as ::text, so pgx returns them as strings already; the assertion
// is defensive against a driver returning something else for an odd key type.
func cursorFromRow(values []any) []string {
	out := make([]string, len(values))
	for i, value := range values {
		if text, ok := value.(string); ok {
			out[i] = text
			continue
		}
		out[i] = fmt.Sprintf("%v", value)
	}
	return out
}

// buildBrowseSQL builds the statement one page runs and the readable statement
// the console shows for it.
//
// It is pure and separate from the execution so the two properties that matter —
// that every identifier it interpolates went through the allowlist, and that the
// keyset comparison matches the ORDER BY — can be read and tested without a
// server. PostgreSQL cannot parameterize an identifier or an ORDER BY column, so
// the schema, table, and key names are interpolated, and quoteIdentifier is the
// whole of the injection defense for them exactly as it is elsewhere in the slice.
//
// The key column types are interpolated into casts (`$1::bigint`) rather than
// quoted. They come from format_type on the catalog, never from the caller, and
// binding the cursor values as text without the cast would make the comparison
// text-against-key and fail on the first non-text key.
func buildBrowseSQL(schema, table string, pk []pkColumn, hasCursor bool) (execSQL, displaySQL string, err error) {
	quotedSchema, err := quoteIdentifier(schema)
	if err != nil {
		return "", "", err
	}
	quotedTable, err := quoteIdentifier(table)
	if err != nil {
		return "", "", err
	}
	relation := quotedSchema + "." + quotedTable

	// One past the page, so a full page signals a next one. See browsePage.
	fetch := strconv.Itoa(browsePageSize + 1)

	if len(pk) == 0 {
		exec := "SELECT * FROM " + relation + " LIMIT " + fetch
		display := "SELECT * FROM " + relation + " LIMIT " + strconv.Itoa(browsePageSize)
		return exec, display, nil
	}

	quotedKeys := make([]string, len(pk))
	casts := make([]string, len(pk))
	cursorCols := make([]string, len(pk))
	for i, column := range pk {
		quoted, keyErr := quoteIdentifier(column.Name)
		if keyErr != nil {
			return "", "", keyErr
		}
		quotedKeys[i] = quoted
		casts[i] = "$" + strconv.Itoa(i+1) + "::" + column.Type
		// Carried out of the page as text so it round-trips through the cursor and
		// casts cleanly back on the next page. Aliased out of the way of a real
		// column; browsePage drops it by position, not by name.
		cursorCols[i] = "t." + quoted + "::text AS __cursor_" + strconv.Itoa(i)
	}
	orderBy := strings.Join(quotedKeys, ", ")

	where := ""
	if hasCursor {
		// Row-value comparison: `(a, b) > ($1, $2)` is the lexicographic step that
		// matches ORDER BY a, b and moves strictly past the last row of the previous
		// page. Its keyset is why the page is stable rather than an OFFSET.
		where = " WHERE (" + orderBy + ") > (" + strings.Join(casts, ", ") + ")"
	}

	exec := "SELECT t.*, " + strings.Join(cursorCols, ", ") +
		" FROM " + relation + " t" + where +
		" ORDER BY " + orderBy + " LIMIT " + fetch
	display := "SELECT * FROM " + relation + " ORDER BY " + orderBy + " LIMIT " + strconv.Itoa(browsePageSize)
	return exec, display, nil
}

// ensureRelationExists refuses a schema and name that is not a browsable relation,
// so the caller gets a not-found rather than an empty page that reads as "the
// table is empty" when the table simply is not there.
func ensureRelationExists(ctx context.Context, tx pgx.Tx, schema, table string) error {
	var relkind string
	err := tx.QueryRow(ctx,
		`SELECT c.relkind
		 FROM pg_catalog.pg_class c
		 JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = $1 AND c.relname = $2`,
		pgx.QueryExecModeExec, schema, table).Scan(&relkind)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: no table or view named %q.%q", ErrNotFound, schema, table)
	}
	if err != nil {
		return fmt.Errorf("look up %q.%q: %w", schema, table, err)
	}
	switch relkind {
	case "r", "p", "v", "m":
		return nil
	default:
		return fmt.Errorf("%w: %q.%q is not a table or view", ErrNotFound, schema, table)
	}
}

// primaryKeyColumns reads a relation's primary key in key order — the order the
// index defines, which is the order the keyset comparison must use, not the order
// the columns happen to sit in the table.
func primaryKeyColumns(ctx context.Context, tx pgx.Tx, schema, table string) ([]pkColumn, error) {
	// unnest ... WITH ORDINALITY walks indkey in key order and numbers each entry
	// from 1, which is both the order the keyset comparison must use and the way to
	// keep only the key columns: the first indnkeyatts of them. The rest are INCLUDE
	// columns — stored in the index but not part of the key — and must not join the
	// comparison. array_position over indkey would do neither cleanly: indkey is an
	// int2vector, which it indexes from zero, so the obvious bound is off by one.
	const query = `
		SELECT a.attname, pg_catalog.format_type(a.atttypid, a.atttypmod)
		FROM pg_catalog.pg_index i
		JOIN pg_catalog.pg_class c ON c.oid = i.indrelid
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		CROSS JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS k(attnum, ord)
		JOIN pg_catalog.pg_attribute a ON a.attrelid = c.oid AND a.attnum = k.attnum
		WHERE i.indisprimary AND n.nspname = $1 AND c.relname = $2 AND k.ord <= i.indnkeyatts
		ORDER BY k.ord`

	rows, err := tx.Query(ctx, query, pgx.QueryExecModeExec, schema, table)
	if err != nil {
		return nil, fmt.Errorf("read the primary key of %q.%q: %w", schema, table, err)
	}
	defer rows.Close()

	columns := []pkColumn{}
	for rows.Next() {
		var column pkColumn
		if err := rows.Scan(&column.Name, &column.Type); err != nil {
			return nil, fmt.Errorf("read a primary-key column: %w", err)
		}
		columns = append(columns, column)
	}
	return columns, rows.Err() //nolint:wrapcheck // rows.Err is already a pgx error with context
}

// encodeCursor packs a row's key values into an opaque token. Base64 of a JSON
// array: the panel treats it as an opaque string and hands it back unchanged, and
// JSON keeps a value that contains the delimiter apart from two values that do not.
func encodeCursor(values []string) string {
	data, _ := json.Marshal(values) // a []string never fails to marshal
	return base64.RawURLEncoding.EncodeToString(data)
}

// decodeCursor reverses encodeCursor, refusing anything this server did not mint.
// A malformed cursor is the caller's bad request, not a server fault.
func decodeCursor(cursor string) ([]string, error) {
	data, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("%w: the page cursor is malformed", ErrInvalidName)
	}
	var values []string
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("%w: the page cursor is malformed", ErrInvalidName)
	}
	return values, nil
}
