package postgres

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

// The relkinds each named-object target may name. GRANT … ON TABLE covers every
// relation kind but a sequence; GRANT … ON SEQUENCE covers only sequences.
var (
	tableRelkinds    = []string{"r", "p", "v", "m", "f"}
	sequenceRelkinds = []string{"S"}
)

// relationKindsQuery reads the kind of each named relation in one schema. A name
// that is not there is simply absent; the GRANT that follows reports it with
// PostgreSQL's own message.
const relationKindsQuery = `
	SELECT c.relname, c.relkind
	FROM pg_catalog.pg_class c
	JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
	WHERE n.nspname = $1 AND c.relname = ANY($2)`

// checkRelationKinds refuses a named table that is a sequence, or a named
// sequence that is not one.
//
// It exists because PostgreSQL does not: GRANT SELECT, INSERT ON TABLE some_seq
// succeeds with a warning and applies SELECT alone, so the request would report
// INSERT granted when it was not. Read in the write transaction, immediately
// before the statement, so what is checked is what the statement acts on.
func checkRelationKinds(ctx context.Context, tx pgx.Tx, change PrivilegeChange) error {
	var allowed []string
	switch change.Target {
	case targetTable:
		allowed = tableRelkinds
	case targetSequence:
		allowed = sequenceRelkinds
	default:
		return nil
	}

	rows, err := tx.Query(ctx, relationKindsQuery, change.Schema, change.Objects)
	if err != nil {
		return fmt.Errorf("read relation kinds: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, relkind string
		if err := rows.Scan(&name, &relkind); err != nil {
			return fmt.Errorf("read a relation kind: %w", err)
		}
		if !slices.Contains(allowed, relkind) {
			return fmt.Errorf("%w: %q is a %s; name it under the %s target",
				ErrInvalidName, name, privilegeObjectKind(relkind), kindTarget(relkind))
		}
	}
	return rows.Err() //nolint:wrapcheck // rows.Err is already a pgx error with context
}

func kindTarget(relkind string) string {
	if relkind == "S" {
		return targetSequence
	}
	return targetTable
}

// The survivor queries find grants to the role ($1) of the revoked privileges
// that are still in an ACL after the REVOKE ran. Because a superuser's REVOKE acts
// as the owner and removes the owner's grants, anything left was granted by
// another role. Each row is (object, privilege, grantor).
const (
	relationSurvivorsQuery = `
	SELECT n.nspname || '.' || c.relname, a.privilege_type, pg_catalog.pg_get_userbyid(a.grantor)
	FROM pg_catalog.pg_class c
	JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
	CROSS JOIN LATERAL pg_catalog.aclexplode(c.relacl) a
	WHERE a.grantee = $1 AND n.nspname = $2 AND c.relkind = ANY($3)
	  AND ($4::text[] IS NULL OR c.relname = ANY($4))
	  AND a.privilege_type = ANY($5)
	ORDER BY 1, 2`

	schemaSurvivorsQuery = `
	SELECT n.nspname, a.privilege_type, pg_catalog.pg_get_userbyid(a.grantor)
	FROM pg_catalog.pg_namespace n
	CROSS JOIN LATERAL pg_catalog.aclexplode(n.nspacl) a
	WHERE a.grantee = $1 AND n.nspname = $2 AND a.privilege_type = ANY($3)
	ORDER BY 1, 2`

	databaseSurvivorsQuery = `
	SELECT d.datname, a.privilege_type, pg_catalog.pg_get_userbyid(a.grantor)
	FROM pg_catalog.pg_database d
	CROSS JOIN LATERAL pg_catalog.aclexplode(d.datacl) a
	WHERE d.datname = pg_catalog.current_database() AND a.grantee = $1 AND a.privilege_type = ANY($2)
	ORDER BY 1, 2`
)

// verifyRevoked checks that a revoke removed what it named, and refuses the whole
// transaction if a grant survived.
//
// The panel does not try to revoke as the recorded grantor. PostgreSQL's REVOKE …
// GRANTED BY accepts only the current user for object privileges, so the only way
// would be SET ROLE to the grantor — running DDL as an arbitrary role chosen by
// what the catalog says, with that role's grant-option state deciding what
// happens. Refusing and naming the grantor is the safer answer: whoever
// administers that role can revoke its grant deliberately, and the panel never
// reports success for a revoke that left the role holding the privilege.
//
// Default privileges need no check: an ALTER DEFAULT PRIVILEGES entry is always
// recorded as granted by its own FOR ROLE, which is the role the revoke names.
func verifyRevoked(ctx context.Context, tx pgx.Tx, roleOID uint32, change PrivilegeChange) error {
	if change.Action != "revoke" {
		return nil
	}
	privileges, err := canonicalPrivileges(privilegeTargets[change.Target].privileges, change.Privileges)
	if err != nil {
		return err
	}

	query, args, ok := survivorQuery(roleOID, change, privileges)
	if !ok {
		return nil
	}
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("check the revoke: %w", err)
	}
	defer rows.Close()

	var survivors []string
	for rows.Next() {
		var object, privilege, grantor string
		if err := rows.Scan(&object, &privilege, &grantor); err != nil {
			return fmt.Errorf("read a surviving grant: %w", err)
		}
		survivors = append(survivors, fmt.Sprintf("%s on %s (granted by %s)", privilege, object, grantor))
	}
	if err := rows.Err(); err != nil {
		return err //nolint:wrapcheck // rows.Err is already a pgx error with context
	}
	if len(survivors) > 0 {
		return fmt.Errorf("%w: %s; a revoke here only removes the owner's grants, so revoke these as "+
			"the role that granted them; nothing was changed", ErrNotRevocable, strings.Join(survivors, ", "))
	}
	return nil
}

// survivorQuery picks the survivor query for a revoke and its arguments. False
// for the default-privileges targets, which need no check.
func survivorQuery(roleOID uint32, change PrivilegeChange, privileges []string) (string, []any, bool) {
	switch change.Target {
	case targetDatabase:
		return databaseSurvivorsQuery, []any{roleOID, privileges}, true
	case targetSchema:
		return schemaSurvivorsQuery, []any{roleOID, change.Schema, privileges}, true
	case targetTable, targetAllTables:
		return relationSurvivorsQuery,
			[]any{roleOID, change.Schema, tableRelkinds, namedOrAll(change), privileges}, true
	case targetSequence, targetAllSequences:
		return relationSurvivorsQuery,
			[]any{roleOID, change.Schema, sequenceRelkinds, namedOrAll(change), privileges}, true
	default:
		return "", nil, false
	}
}

// namedOrAll is the relation-name filter for a survivor query: the named objects,
// or nil — NULL to the query — for an "all in schema" target.
func namedOrAll(change PrivilegeChange) []string {
	if change.Target == targetAllTables || change.Target == targetAllSequences {
		return nil
	}
	return change.Objects
}
