package postgres

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// commitTimeout bounds the COMMIT alone, separately from the statement's deadline,
// so acknowledging a commit always has time even after a long statement.
const commitTimeout = 10 * time.Second

// Execute runs one statement in a read-write transaction and commits it.
//
// This is where the read-only console's guarantee is deliberately dropped: the
// transaction is not READ ONLY, and it is committed on success rather than always
// rolled back, so an INSERT, UPDATE, DELETE or DDL persists. Everything else is
// the same as Query — a short-lived connection, a statement timeout, and one
// statement per message via the extended protocol, so `UPDATE …; DROP TABLE t` is
// still refused rather than run as two.
//
// The caller reaches this only through the write-authorized action, so the bound
// that matters is who is asking, not what the statement is. As with Query, nothing
// here inspects the SQL: PostgreSQL runs it, and a pattern match would suggest a
// boundary this does not have.
func (r *Repository) Execute(ctx context.Context, database, sql string) (ExecuteResult, error) {
	ctx, cancel := context.WithTimeout(ctx, queryDeadline)
	defer cancel()

	conn, err := r.connectTo(ctx, database)
	if err != nil {
		return ExecuteResult{}, err
	}
	defer func() { _ = conn.Close(ctx) }()

	// A read-write transaction, unlike Query's. Committed only on the success path
	// below; the deferred rollback covers every early return, and is a no-op once
	// the commit has happened.
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("begin a transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	if _, err := tx.Exec(ctx,
		"SELECT set_config('statement_timeout', $1, true)",
		strconv.FormatInt(queryTimeout.Milliseconds(), 10)); err != nil {
		return ExecuteResult{}, fmt.Errorf("bound the statement: %w", err)
	}

	started := time.Now()
	rows, err := tx.Query(ctx, sql, pgx.QueryExecModeExec)
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("%w: %s", ErrQueryFailed, err.Error())
	}

	// Collect any rows the statement returned — a RETURNING clause, or a plain
	// SELECT run through here — reusing the query path's renderer and cap.
	collected, err := collectRows(rows)
	if err != nil {
		rows.Close()
		return ExecuteResult{}, err
	}
	// CommandTag is only valid once the rows are closed. It carries the tag —
	// "UPDATE 3" — and the affected count, which is the whole answer for a write
	// that returns nothing, and stays exact even when the RETURNING rows were capped.
	rows.Close()
	tag := rows.CommandTag()

	// Commit under its own deadline, detached from the statement's. A statement
	// that used most of queryDeadline must not leave the commit without time to be
	// acknowledged: a COMMIT PostgreSQL applied but pgx could not confirm — because
	// the shared deadline expired waiting for the ack — comes back as an error, and
	// a retry of a non-idempotent statement would then apply it twice. WithoutCancel
	// keeps the connection's context values while dropping the spent deadline.
	commitCtx, cancelCommit := context.WithTimeout(context.WithoutCancel(ctx), commitTimeout)
	defer cancelCommit()
	if err := tx.Commit(commitCtx); err != nil {
		return ExecuteResult{}, fmt.Errorf("commit the statement: %w", err)
	}
	committed = true

	return ExecuteResult{
		Columns:      collected.Columns,
		Rows:         collected.Rows,
		Truncated:    collected.Truncated,
		Command:      tag.String(),
		RowsAffected: tag.RowsAffected(),
		ElapsedMs:    time.Since(started).Milliseconds(),
	}, nil
}
