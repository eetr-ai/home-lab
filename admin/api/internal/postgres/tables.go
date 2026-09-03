package postgres

import (
	"context"
	"fmt"
)

// relationsQuery lists the columns of every browsable relation in one database,
// one row per column, ordered so a relation's columns arrive together and in
// their declared order.
//
// pg_catalog rather than information_schema: format_type renders the declared
// type the way PostgreSQL itself prints it, including the length modifier, and
// the catalogs carry relkind and the primary-key flag without another join per
// column. The system schemas are excluded because nobody administers a lab by
// reading pg_catalog through this panel, and toast and temp schemas hold nothing
// a person names.
const relationsQuery = `
	SELECT n.nspname,
	       c.relname,
	       c.relkind,
	       a.attname,
	       pg_catalog.format_type(a.atttypid, a.atttypmod),
	       NOT a.attnotnull,
	       EXISTS (
	           -- A primary-key column, but only among the key columns: indkey's first
	           -- indnkeyatts entries. A trailing INCLUDE column is stored in the index
	           -- yet is not part of the key. WITH ORDINALITY numbers the entries from 1
	           -- so the bound is exact; array_position would index int2vector from 0.
	           SELECT 1 FROM pg_catalog.pg_index i
	           CROSS JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS k(attnum, ord)
	           WHERE i.indrelid = c.oid AND i.indisprimary
	             AND k.attnum = a.attnum AND k.ord <= i.indnkeyatts
	       )
	FROM pg_catalog.pg_class c
	JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
	JOIN pg_catalog.pg_attribute a ON a.attrelid = c.oid
	WHERE c.relkind IN ('r', 'p', 'v', 'm')
	  AND a.attnum > 0 AND NOT a.attisdropped
	  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
	  AND n.nspname NOT LIKE 'pg\_toast%'
	  AND n.nspname NOT LIKE 'pg\_temp%'
	ORDER BY n.nspname, c.relname, a.attnum`

// ListRelations returns the tables and views in one database, each with its
// columns.
func (r *Repository) ListRelations(ctx context.Context, database string) ([]Relation, error) {
	// The same overall bound Query and Browse get from inReadOnlyTx, for the same
	// reason: an incoming request context carries no deadline, so a connect to an
	// unresponsive server or a slow catalog scan would otherwise hold the handler
	// goroutine with nothing to stop it.
	ctx, cancel := context.WithTimeout(ctx, queryDeadline)
	defer cancel()

	conn, err := r.connectTo(ctx, database)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close(ctx) }()

	rows, err := conn.Query(ctx, relationsQuery)
	if err != nil {
		return nil, fmt.Errorf("list relations in %q: %w", database, err)
	}
	defer rows.Close()

	// One relation is a run of consecutive rows sharing a schema and name, because
	// the query orders by both. A new (schema, name) starts a new relation; every
	// other row extends the last. Tracked by index rather than a pointer into the
	// slice, because appending a later relation can move the backing array and
	// leave a held pointer writing to the old one.
	relations := []Relation{}
	for rows.Next() {
		var (
			schema, name, relkind, column, columnType string
			nullable, primaryKey                       bool
		)
		if err := rows.Scan(&schema, &name, &relkind, &column, &columnType, &nullable, &primaryKey); err != nil {
			return nil, fmt.Errorf("read a relation row: %w", err)
		}
		last := len(relations) - 1
		if last < 0 || relations[last].Schema != schema || relations[last].Name != name {
			relations = append(relations, Relation{Schema: schema, Name: name, Kind: relationKind(relkind), Columns: []Column{}})
			last = len(relations) - 1
		}
		relations[last].Columns = append(relations[last].Columns, Column{
			Name:       column,
			Type:       columnType,
			Nullable:   nullable,
			PrimaryKey: primaryKey,
		})
	}
	return relations, rows.Err() //nolint:wrapcheck // rows.Err is already a pgx error with context
}

// relationKind maps a pg_class.relkind to the word the panel shows. A partitioned
// table is a table; an ordinary and a materialized view are told apart because a
// matview holds rows of its own and the distinction is worth seeing.
func relationKind(relkind string) string {
	switch relkind {
	case "v":
		return "view"
	case "m":
		return "matview"
	default: // "r", "p"
		return "table"
	}
}
