package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// The listing queries all take the role's OID as $1 and the privileges to test as
// $2, and return, per object, three arrays: the privileges granted to the role by
// name, the ones granted to it by somebody other than the object's owner (as
// "PRIVILEGE:grantor"), and the ones it effectively holds.
//
// Granted reads the object's ACL with aclexplode. A NULL ACL is not "nothing
// granted" — it means the object still has its built-in default, which grants
// the owner everything — so the ACL is coalesced with acldefault for the object's
// kind first. Without that, an owner would be listed as holding nothing on its
// own tables until somebody first granted on them.
//
// The grantor matters because of how REVOKE works when a superuser runs it: it
// acts as the object's owner, and removes only the grants the owner made. A grant
// some other role made — one that held the privilege WITH GRANT OPTION — survives
// it. Those are reported separately so the panel can say so rather than offer a
// revoke that would quietly do nothing.
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
	       ARRAY(SELECT a.privilege_type || ':' || pg_catalog.pg_get_userbyid(a.grantor)
	             FROM pg_catalog.aclexplode(d.datacl) a
	             WHERE a.grantee = $1 AND a.grantor <> d.datdba),
	       ARRAY(SELECT p FROM unnest($2::text[]) p
	             WHERE pg_catalog.has_database_privilege($1, d.oid, p))
	FROM pg_catalog.pg_database d
	WHERE d.datname = pg_catalog.current_database()`

	schemaPrivilegesQuery = `
	SELECT n.nspname, pg_catalog.pg_get_userbyid(n.nspowner),
	       ARRAY(SELECT a.privilege_type
	             FROM pg_catalog.aclexplode(COALESCE(n.nspacl, pg_catalog.acldefault('n', n.nspowner))) a
	             WHERE a.grantee = $1),
	       ARRAY(SELECT a.privilege_type || ':' || pg_catalog.pg_get_userbyid(a.grantor)
	             FROM pg_catalog.aclexplode(n.nspacl) a
	             WHERE a.grantee = $1 AND a.grantor <> n.nspowner),
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
	       ARRAY(SELECT a.privilege_type || ':' || pg_catalog.pg_get_userbyid(a.grantor)
	             FROM pg_catalog.aclexplode(c.relacl) a
	             WHERE a.grantee = $1 AND a.grantor <> c.relowner),
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
	       ARRAY(SELECT a.privilege_type || ':' || pg_catalog.pg_get_userbyid(a.grantor)
	             FROM pg_catalog.aclexplode(c.relacl) a
	             WHERE a.grantee = $1 AND a.grantor <> c.relowner),
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
// live in its own catalog and cannot be read from anywhere else. The transaction
// is REPEATABLE READ (see inReadOnlyTx), so the five reads below see one snapshot:
// a preset committed between two of them cannot produce a listing that shows its
// USAGE on the schema but not its grants on the tables.
func (r *Repository) ListPrivileges(ctx context.Context, database, role string) (RolePrivileges, error) {
	var result RolePrivileges
	err := r.inReadOnlyTx(ctx, database, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		result, err = readRolePrivileges(ctx, tx, database, role)
		return err
	})
	return result, err
}

// readRolePrivileges is the listing, on a transaction the caller owns. Shared by
// ListPrivileges and ApplyPrivileges, which reads the state it leaves behind
// before committing it.
func readRolePrivileges(ctx context.Context, tx pgx.Tx, database, role string) (RolePrivileges, error) {
	result := RolePrivileges{Role: role, Database: database}
	roleOID, err := lookupRoleOID(ctx, tx, role)
	if err != nil {
		return RolePrivileges{}, err
	}

	if result.DatabasePrivileges, err = readDatabasePrivileges(ctx, tx, roleOID); err != nil {
		return RolePrivileges{}, err
	}
	result.Schemas, err = readObjectPrivileges(ctx, tx, schemaPrivilegesQuery, roleOID, schemaPrivileges, false)
	if err != nil {
		return RolePrivileges{}, err
	}
	result.Tables, err = readObjectPrivileges(ctx, tx, tablePrivilegesQuery, roleOID, tablePrivileges, true)
	if err != nil {
		return RolePrivileges{}, err
	}
	result.Sequences, err = readObjectPrivileges(ctx, tx, sequencePrivilegesQuery, roleOID, sequencePrivileges, true)
	if err != nil {
		return RolePrivileges{}, err
	}
	if result.Defaults, err = readDefaultPrivileges(ctx, tx, roleOID); err != nil {
		return RolePrivileges{}, err
	}
	return result, nil
}

// lookupRoleOID resolves a role name. The OID, not the name, is what aclexplode
// reports a grantee as. Roles are cluster-wide, so it is the same in every
// database.
func lookupRoleOID(ctx context.Context, tx pgx.Tx, role string) (uint32, error) {
	var roleOID uint32
	if err := tx.QueryRow(ctx,
		"SELECT oid FROM pg_catalog.pg_roles WHERE rolname = $1", role).Scan(&roleOID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("%w: no role named %q", ErrNotFound, role)
		}
		return 0, fmt.Errorf("look up role %q: %w", role, err)
	}
	return roleOID, nil
}

func readDatabasePrivileges(ctx context.Context, tx pgx.Tx, roleOID uint32) (ObjectPrivileges, error) {
	object := ObjectPrivileges{Kind: targetDatabase}
	var granted, foreign, effective []string
	if err := tx.QueryRow(ctx, databasePrivilegesQuery, roleOID, databasePrivileges).
		Scan(&object.Name, &object.Owner, &granted, &foreign, &effective); err != nil {
		return ObjectPrivileges{}, fmt.Errorf("read database privileges: %w", err)
	}
	object.Granted = orderPrivileges(databasePrivileges, granted)
	object.ForeignGrants = parseForeignGrants(databasePrivileges, foreign)
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
			object                      ObjectPrivileges
			relkind                     string
			granted, foreign, effective []string
		)
		if qualified {
			err = rows.Scan(&object.Schema, &object.Name, &relkind, &object.Owner, &granted, &foreign, &effective)
			object.Kind = privilegeObjectKind(relkind)
		} else {
			err = rows.Scan(&object.Name, &object.Owner, &granted, &foreign, &effective)
			object.Kind = targetSchema
		}
		if err != nil {
			return nil, fmt.Errorf("read a privilege row: %w", err)
		}
		object.Granted = orderPrivileges(allowed, granted)
		object.ForeignGrants = parseForeignGrants(allowed, foreign)
		object.Effective = orderPrivileges(allowed, effective)
		objects = append(objects, object)
	}
	return objects, rows.Err() //nolint:wrapcheck // rows.Err is already a pgx error with context
}

// parseForeignGrants reads the "PRIVILEGE:grantor" pairs the listing queries
// build. Split on the first colon: a privilege keyword never contains one, and a
// grantor's name may. Privileges the panel does not show are dropped, the same as
// in every other list.
func parseForeignGrants(allowed, pairs []string) []ForeignGrant {
	grants := []ForeignGrant{}
	for _, privilege := range allowed {
		for _, pair := range pairs {
			name, grantor, ok := strings.Cut(pair, ":")
			if ok && name == privilege {
				grants = append(grants, ForeignGrant{Privilege: name, Grantor: grantor})
			}
		}
	}
	return grants
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
		// An entry holding only privileges the panel does not show — MAINTAIN
		// alone, on PostgreSQL 17 and later — is left out rather than listed with
		// nothing in it: its revoke would send an empty list, which is refused.
		if len(entry.Privileges) == 0 {
			continue
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
// a connection to the database they apply in, and returns the role's privileges
// as that transaction leaves them.
//
// changes and statements are parallel: statements[i] is what privilegeStatements
// built from changes[i]. The statements are what runs; the changes are what this
// checks before and after, because two things a statement can do are not what was
// asked, and PostgreSQL reports neither as an error:
//
//   - GRANT … ON TABLE accepts a sequence, warns, and applies only the privileges
//     that make sense for one. A named table that is really a sequence is refused
//     before anything runs.
//   - REVOKE run by a superuser acts as the owner and removes only the owner's
//     grants. A grant some other role made survives, and the role keeps the
//     privilege. After the statements run, every revoke is checked against the
//     catalog, and one that left a grant behind fails the whole transaction,
//     naming the grantor.
//
// One transaction so a preset lands whole: read-write access that granted USAGE
// on the schema and then failed on the tables would leave a role that can see a
// schema it cannot read, with nothing saying so. The returned state is read inside
// that transaction, before the commit, so a successful change can never be
// reported as a failure merely because reading it back afterwards failed.
func (r *Repository) ApplyPrivileges(
	ctx context.Context, database, role string, changes []PrivilegeChange, statements []string,
) (RolePrivileges, error) {
	if len(changes) != len(statements) {
		return RolePrivileges{}, fmt.Errorf("%d changes but %d statements", len(changes), len(statements))
	}

	ctx, cancel := context.WithTimeout(ctx, queryDeadline)
	defer cancel()

	conn, err := r.connectTo(ctx, database)
	if err != nil {
		return RolePrivileges{}, err
	}
	defer func() { _ = conn.Close(ctx) }()

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RolePrivileges{}, fmt.Errorf("begin a transaction: %w", err)
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
		return RolePrivileges{}, fmt.Errorf("bound the statements: %w", err)
	}

	state, err := applyInTx(ctx, tx, database, role, changes, statements)
	if err != nil {
		return RolePrivileges{}, err
	}

	// Committed under its own deadline for the reason Execute gives: a commit the
	// server applied but the client could not confirm reads as a failure.
	commitCtx, cancelCommit := context.WithTimeout(context.WithoutCancel(ctx), commitTimeout)
	defer cancelCommit()
	if err := tx.Commit(commitCtx); err != nil {
		return RolePrivileges{}, fmt.Errorf("commit the privilege changes: %w", err)
	}
	committed = true
	return state, nil
}

// applyInTx is ApplyPrivileges' work inside the transaction: check, run, verify,
// and read back.
func applyInTx(
	ctx context.Context, tx pgx.Tx, database, role string, changes []PrivilegeChange, statements []string,
) (RolePrivileges, error) {
	roleOID, err := lookupRoleOID(ctx, tx, role)
	if err != nil {
		return RolePrivileges{}, err
	}

	for i, change := range changes {
		if err := checkRelationKinds(ctx, tx, change); err != nil {
			return RolePrivileges{}, fmt.Errorf("change %d: %w", i+1, err)
		}
		// A statement the server refuses — a table that has gone since the page
		// was loaded, say — is reported with PostgreSQL's own message, which names
		// the object. That is the useful part, and the same contract as the console's.
		if _, err := tx.Exec(ctx, statements[i]); err != nil {
			return RolePrivileges{}, fmt.Errorf("%w: %s", ErrQueryFailed, err.Error())
		}
	}

	for i, change := range changes {
		if err := verifyRevoked(ctx, tx, roleOID, change); err != nil {
			return RolePrivileges{}, fmt.Errorf("change %d: %w", i+1, err)
		}
	}

	return readRolePrivileges(ctx, tx, database, role)
}
