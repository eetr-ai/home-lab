package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// The listing queries all take the role's OID as $1 and the privileges to test as
// $2, and return, per object, the privileges granted to the role by name and the
// ones it effectively holds.
//
// Granted reads the object's ACL with aclexplode. A NULL ACL is not "nothing
// granted" — it means the object still has its built-in default, which gives the
// owner everything — so the ACL is coalesced with acldefault for the object's
// kind first. Without that, an owner would be listed as holding nothing on its
// own tables until somebody first granted on them.
//
// Effective asks has_*_privilege, which is PostgreSQL's own answer: it counts
// PUBLIC, inherited role memberships, ownership and superuser. The two are asked
// separately because only the first is something a REVOKE from this role removes.
//
// The system schemas are left out for the same reason the console's schema tree
// leaves them out: nobody grants an application role rights on pg_catalog.
const (
	privilegeSchemaFilter = `
	      n.nspname NOT IN ('pg_catalog', 'information_schema')
	  AND n.nspname NOT LIKE 'pg\_toast%'
	  AND n.nspname NOT LIKE 'pg\_temp%'`

	databasePrivilegesQuery = `
	SELECT d.datname, pg_catalog.pg_get_userbyid(d.datdba),
	       ARRAY(SELECT a.privilege_type
	             FROM pg_catalog.aclexplode(COALESCE(d.datacl, pg_catalog.acldefault('d', d.datdba))) a
	             WHERE a.grantee = $1),
	       ARRAY(SELECT p FROM unnest($2::text[]) p
	             WHERE pg_catalog.has_database_privilege($1, d.oid, p))
	FROM pg_catalog.pg_database d
	WHERE d.datname = pg_catalog.current_database()`

	schemaPrivilegesQuery = `
	SELECT n.nspname, pg_catalog.pg_get_userbyid(n.nspowner),
	       ARRAY(SELECT a.privilege_type
	             FROM pg_catalog.aclexplode(COALESCE(n.nspacl, pg_catalog.acldefault('n', n.nspowner))) a
	             WHERE a.grantee = $1),
	       ARRAY(SELECT p FROM unnest($2::text[]) p
	             WHERE pg_catalog.has_schema_privilege($1, n.oid, p))
	FROM pg_catalog.pg_namespace n
	WHERE` + privilegeSchemaFilter + `
	ORDER BY n.nspname`

	// Tables, views, materialized views, foreign tables and partitioned tables:
	// everything GRANT … ON TABLE applies to. acldefault's 'r' covers them all.
	tablePrivilegesQuery = `
	SELECT n.nspname, c.relname, c.relkind, pg_catalog.pg_get_userbyid(c.relowner),
	       ARRAY(SELECT a.privilege_type
	             FROM pg_catalog.aclexplode(COALESCE(c.relacl, pg_catalog.acldefault('r', c.relowner))) a
	             WHERE a.grantee = $1),
	       ARRAY(SELECT p FROM unnest($2::text[]) p
	             WHERE pg_catalog.has_table_privilege($1, c.oid, p))
	FROM pg_catalog.pg_class c
	JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
	WHERE c.relkind IN ('r', 'p', 'v', 'm', 'f') AND` + privilegeSchemaFilter + `
	ORDER BY n.nspname, c.relname`

	sequencePrivilegesQuery = `
	SELECT n.nspname, c.relname, c.relkind, pg_catalog.pg_get_userbyid(c.relowner),
	       ARRAY(SELECT a.privilege_type
	             FROM pg_catalog.aclexplode(COALESCE(c.relacl, pg_catalog.acldefault('s', c.relowner))) a
	             WHERE a.grantee = $1),
	       ARRAY(SELECT p FROM unnest($2::text[]) p
	             WHERE pg_catalog.has_sequence_privilege($1, c.oid, p))
	FROM pg_catalog.pg_class c
	JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
	WHERE c.relkind = 'S' AND` + privilegeSchemaFilter + `
	ORDER BY n.nspname, c.relname`

	// Only entries that name the role. An entry with namespace 0 applies in every
	// schema, and the LEFT JOIN leaves its schema NULL, read back as empty.
	defaultPrivilegesQuery = `
	SELECT pg_catalog.pg_get_userbyid(d.defaclrole), COALESCE(n.nspname, ''), d.defaclobjtype,
	       ARRAY(SELECT a.privilege_type FROM pg_catalog.aclexplode(d.defaclacl) a
	             WHERE a.grantee = $1)
	FROM pg_catalog.pg_default_acl d
	LEFT JOIN pg_catalog.pg_namespace n ON n.oid = d.defaclnamespace
	WHERE d.defaclobjtype IN ('r', 'S')
	  AND EXISTS (SELECT 1 FROM pg_catalog.aclexplode(d.defaclacl) a WHERE a.grantee = $1)
	ORDER BY 1, 2, 3`
)

// ListPrivileges reads what one role holds in one database.
//
// It runs inside the same rolled-back READ ONLY transaction as the query console,
// on a connection to that database: the ACLs of its schemas, tables and sequences
// live in its own catalog and cannot be read from anywhere else.
func (r *Repository) ListPrivileges(ctx context.Context, database, role string) (RolePrivileges, error) {
	result := RolePrivileges{Role: role, Database: database}
	err := r.inReadOnlyTx(ctx, database, func(ctx context.Context, tx pgx.Tx) error {
		// The OID, not the name, is what aclexplode reports a grantee as. Roles are
		// cluster-wide, so it is the same in every database.
		var roleOID uint32
		if err := tx.QueryRow(ctx,
			"SELECT oid FROM pg_catalog.pg_roles WHERE rolname = $1", role).Scan(&roleOID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("%w: no role named %q", ErrNotFound, role)
			}
			return fmt.Errorf("look up role %q: %w", role, err)
		}

		var err error
		if result.DatabasePrivileges, err = readDatabasePrivileges(ctx, tx, roleOID); err != nil {
			return err
		}
		result.Schemas, err = readObjectPrivileges(ctx, tx, schemaPrivilegesQuery, roleOID, schemaPrivileges, false)
		if err != nil {
			return err
		}
		result.Tables, err = readObjectPrivileges(ctx, tx, tablePrivilegesQuery, roleOID, tablePrivileges, true)
		if err != nil {
			return err
		}
		result.Sequences, err = readObjectPrivileges(ctx, tx, sequencePrivilegesQuery, roleOID, sequencePrivileges, true)
		if err != nil {
			return err
		}
		result.Defaults, err = readDefaultPrivileges(ctx, tx, roleOID)
		return err
	})
	return result, err
}

func readDatabasePrivileges(ctx context.Context, tx pgx.Tx, roleOID uint32) (ObjectPrivileges, error) {
	object := ObjectPrivileges{Kind: targetDatabase}
	var granted, effective []string
	if err := tx.QueryRow(ctx, databasePrivilegesQuery, roleOID, databasePrivileges).
		Scan(&object.Name, &object.Owner, &granted, &effective); err != nil {
		return ObjectPrivileges{}, fmt.Errorf("read database privileges: %w", err)
	}
	object.Granted = orderPrivileges(databasePrivileges, granted)
	object.Effective = orderPrivileges(databasePrivileges, effective)
	return object, nil
}

// readObjectPrivileges runs one of the schema, table or sequence queries. The
// relation queries carry a schema and a relkind ahead of the owner; the schema
// query does not, which is the one difference qualified accounts for.
func readObjectPrivileges(
	ctx context.Context, tx pgx.Tx, query string, roleOID uint32, allowed []string, qualified bool,
) ([]ObjectPrivileges, error) {
	rows, err := tx.Query(ctx, query, roleOID, allowed)
	if err != nil {
		return nil, fmt.Errorf("read privileges: %w", err)
	}
	defer rows.Close()

	objects := []ObjectPrivileges{}
	for rows.Next() {
		var (
			object             ObjectPrivileges
			relkind            string
			granted, effective []string
		)
		if qualified {
			err = rows.Scan(&object.Schema, &object.Name, &relkind, &object.Owner, &granted, &effective)
			object.Kind = privilegeObjectKind(relkind)
		} else {
			err = rows.Scan(&object.Name, &object.Owner, &granted, &effective)
			object.Kind = targetSchema
		}
		if err != nil {
			return nil, fmt.Errorf("read a privilege row: %w", err)
		}
		object.Granted = orderPrivileges(allowed, granted)
		object.Effective = orderPrivileges(allowed, effective)
		objects = append(objects, object)
	}
	return objects, rows.Err() //nolint:wrapcheck // rows.Err is already a pgx error with context
}

func readDefaultPrivileges(ctx context.Context, tx pgx.Tx, roleOID uint32) ([]DefaultPrivileges, error) {
	rows, err := tx.Query(ctx, defaultPrivilegesQuery, roleOID)
	if err != nil {
		return nil, fmt.Errorf("read default privileges: %w", err)
	}
	defer rows.Close()

	defaults := []DefaultPrivileges{}
	for rows.Next() {
		var (
			entry      DefaultPrivileges
			objectType string
			privileges []string
		)
		if err := rows.Scan(&entry.Owner, &entry.Schema, &objectType, &privileges); err != nil {
			return nil, fmt.Errorf("read a default privilege row: %w", err)
		}
		if objectType == "S" {
			entry.ObjectType = "sequences"
			entry.Privileges = orderPrivileges(sequencePrivileges, privileges)
		} else {
			entry.ObjectType = "tables"
			entry.Privileges = orderPrivileges(tablePrivileges, privileges)
		}
		defaults = append(defaults, entry)
	}
	return defaults, rows.Err() //nolint:wrapcheck // rows.Err is already a pgx error with context
}

// privilegeObjectKind maps a relkind to the word the panel shows.
func privilegeObjectKind(relkind string) string {
	switch relkind {
	case "S":
		return targetSequence
	case "f":
		return "foreign"
	default:
		return relationKind(relkind)
	}
}

// ApplyPrivileges runs a set of GRANT and REVOKE statements in one transaction on
// a connection to the database they apply in.
//
// One transaction so a preset lands whole: read-write access that granted USAGE
// on the schema and then failed on the tables would leave a role that can see a
// schema it cannot read, with nothing saying so. The statements were built by
// privilegeStatements from validated, quoted input; nothing here inspects them.
//
// A statement the server refuses — a table that has gone since the page was
// loaded, say — is reported with PostgreSQL's own message, which names the
// object. That is the useful part, and it is the same contract as the console's.
func (r *Repository) ApplyPrivileges(ctx context.Context, database string, statements []string) error {
	ctx, cancel := context.WithTimeout(ctx, queryDeadline)
	defer cancel()

	conn, err := r.connectTo(ctx, database)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin a transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	// A GRANT on a table waits for any lock that conflicts with it, so a long
	// transaction elsewhere would otherwise hold this request open indefinitely.
	if _, err := tx.Exec(ctx,
		"SELECT set_config('statement_timeout', $1, true)",
		strconv.FormatInt(queryTimeout.Milliseconds(), 10)); err != nil {
		return fmt.Errorf("bound the statements: %w", err)
	}

	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("%w: %s", ErrQueryFailed, err.Error())
		}
	}

	// Committed under its own deadline for the reason Execute gives: a commit the
	// server applied but the client could not confirm reads as a failure.
	commitCtx, cancelCommit := context.WithTimeout(context.WithoutCancel(ctx), commitTimeout)
	defer cancelCommit()
	if err := tx.Commit(commitCtx); err != nil {
		return fmt.Errorf("commit the privilege changes: %w", err)
	}
	committed = true
	return nil
}
