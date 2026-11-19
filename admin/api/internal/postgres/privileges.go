package postgres

import (
	"fmt"
	"slices"
	"strings"
)

// maxPrivilegeChanges bounds one request. A preset is four or five changes and a
// page of ticked boxes a few dozen; this is generous for both and keeps one
// request from becoming a transaction holding locks on hundreds of tables.
const maxPrivilegeChanges = 100

// maxPrivilegeObjects bounds the tables or sequences one change names.
const maxPrivilegeObjects = 200

// The privileges the panel shows and accepts, per kind of object, in the order
// PostgreSQL's own documentation lists them. This is the allowlist: a keyword not
// here never reaches a statement, so the privilege list is as inert as a quoted
// name.
//
// MAINTAIN (PostgreSQL 17 and later) is left out of the table list on purpose.
// has_table_privilege raises on an older server when asked about it, and a
// listing that fails outright on one version is worse than one that leaves out a
// privilege no application role in this lab needs.
var (
	databasePrivileges = []string{"CONNECT", "CREATE", "TEMPORARY"}
	schemaPrivileges   = []string{"USAGE", "CREATE"}
	tablePrivileges    = []string{"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"}
	sequencePrivileges = []string{"USAGE", "SELECT", "UPDATE"}
)

// The values of PrivilegeChange.Target. The first four double as the Kind of an
// ObjectPrivileges in a listing, which is why a listing's kinds and a change's
// targets read the same.
const (
	targetDatabase         = "database"
	targetSchema           = "schema"
	targetTable            = "table"
	targetSequence         = "sequence"
	targetAllTables        = "allTables"
	targetAllSequences     = "allSequences"
	targetDefaultTables    = "defaultTables"
	targetDefaultSequences = "defaultSequences"
)

// privilegeTarget describes one value of PrivilegeChange.Target: which privileges
// it accepts and which of the other fields it reads.
type privilegeTarget struct {
	privileges   []string
	needsSchema  bool // Schema is required
	allowsSchema bool // Schema may be given (default targets: optional)
	needsObjects bool
	needsOwner   bool
}

var privilegeTargets = map[string]privilegeTarget{
	targetDatabase:         {privileges: databasePrivileges},
	targetSchema:           {privileges: schemaPrivileges, needsSchema: true},
	targetTable:            {privileges: tablePrivileges, needsSchema: true, needsObjects: true},
	targetSequence:         {privileges: sequencePrivileges, needsSchema: true, needsObjects: true},
	targetAllTables:        {privileges: tablePrivileges, needsSchema: true},
	targetAllSequences:     {privileges: sequencePrivileges, needsSchema: true},
	targetDefaultTables:    {privileges: tablePrivileges, allowsSchema: true, needsOwner: true},
	targetDefaultSequences: {privileges: sequencePrivileges, allowsSchema: true, needsOwner: true},
}

// privilegeStatements builds the GRANT, REVOKE and ALTER DEFAULT PRIVILEGES
// statements for a set of changes, kept separate — and pure — so the rules can be
// tested without a server.
//
// The database and role are validated with the strict allowlist, like every role
// and database name in this slice. Schema, table and sequence names go through
// quoteName instead: they name objects that already exist and that the panel
// listed, and a real table may be capitalised or spaced. The default-privileges
// owner is a role, so it gets the allowlist.
func privilegeStatements(database, role string, changes []PrivilegeChange) ([]string, error) {
	if len(changes) == 0 {
		return nil, fmt.Errorf("%w: there are no changes to make", ErrInvalidName)
	}
	if len(changes) > maxPrivilegeChanges {
		return nil, fmt.Errorf("%w: at most %d changes may be made at once", ErrInvalidName, maxPrivilegeChanges)
	}
	quotedDatabase, err := quoteIdentifier(database)
	if err != nil {
		return nil, err
	}
	quotedRole, err := quoteIdentifier(role)
	if err != nil {
		return nil, fmt.Errorf("role: %w", err)
	}

	statements := make([]string, 0, len(changes))
	for i, change := range changes {
		statement, err := privilegeStatement(quotedDatabase, quotedRole, change)
		if err != nil {
			// Numbered, because "an unknown privilege" in a batch of twelve is not
			// an answer anybody can act on.
			return nil, fmt.Errorf("change %d: %w", i+1, err)
		}
		statements = append(statements, statement)
	}
	return statements, nil
}

// privilegeStatement builds one statement. Its inputs are already-quoted database
// and role names; everything in the change is validated and quoted here.
func privilegeStatement(quotedDatabase, quotedRole string, change PrivilegeChange) (string, error) {
	target, ok := privilegeTargets[change.Target]
	if !ok {
		return "", fmt.Errorf("%w: %q is not something privileges can be granted on", ErrInvalidName, change.Target)
	}
	if err := checkTargetFields(target, change); err != nil {
		return "", err
	}
	privileges, err := canonicalPrivileges(target.privileges, change.Privileges)
	if err != nil {
		return "", err
	}

	// GRANT … TO, REVOKE … FROM. Never WITH GRANT OPTION and never CASCADE: the
	// panel does not hand out the right to re-grant, so there is nothing for a
	// cascade to reach, and RESTRICT (the default) failing loudly would mean
	// somebody else did.
	var verb, preposition string
	switch change.Action {
	case "grant":
		verb, preposition = "GRANT", "TO"
	case "revoke":
		verb, preposition = "REVOKE", "FROM"
	default:
		return "", fmt.Errorf("%w: the action is %q; it must be grant or revoke", ErrInvalidName, change.Action)
	}
	clause := verb + " " + strings.Join(privileges, ", ")

	if target.needsOwner {
		return defaultPrivilegesStatement(change, clause, preposition, quotedRole)
	}

	object, err := privilegeObject(quotedDatabase, change)
	if err != nil {
		return "", err
	}
	return clause + " ON " + object + " " + preposition + " " + quotedRole, nil
}

// checkTargetFields refuses a field the target does not read, and requires the
// ones it does. Refused rather than ignored, for the reason the rest of this
// slice gives: a caller who sent a list of tables with "allTables" believed the
// list meant something.
func checkTargetFields(target privilegeTarget, change PrivilegeChange) error {
	switch {
	case target.needsSchema && change.Schema == "":
		return fmt.Errorf("%w: %s needs a schema", ErrInvalidName, change.Target)
	case !target.needsSchema && !target.allowsSchema && change.Schema != "":
		return fmt.Errorf("%w: %s takes no schema", ErrInvalidName, change.Target)
	case target.needsOwner && change.Owner == "":
		return fmt.Errorf("%w: %s needs the owner whose future objects it covers", ErrInvalidName, change.Target)
	case !target.needsOwner && change.Owner != "":
		return fmt.Errorf("%w: %s takes no owner", ErrInvalidName, change.Target)
	}
	return checkTargetObjects(target, change)
}

// checkTargetObjects is checkTargetFields for the list of objects, split out only
// to keep each switch short enough to read at a glance.
func checkTargetObjects(target privilegeTarget, change PrivilegeChange) error {
	switch {
	case target.needsObjects && len(change.Objects) == 0:
		return fmt.Errorf("%w: %s needs at least one object", ErrInvalidName, change.Target)
	case !target.needsObjects && len(change.Objects) > 0:
		return fmt.Errorf("%w: %s takes no list of objects", ErrInvalidName, change.Target)
	case len(change.Objects) > maxPrivilegeObjects:
		return fmt.Errorf("%w: at most %d objects may be named in one change", ErrInvalidName, maxPrivilegeObjects)
	}
	return nil
}

// canonicalPrivileges checks every requested privilege against the target's
// allowlist and returns them upper-cased, de-duplicated, and in the allowlist's
// order — so the same request always produces the same statement.
func canonicalPrivileges(allowed, requested []string) ([]string, error) {
	if len(requested) == 0 {
		return nil, fmt.Errorf("%w: name at least one privilege", ErrInvalidName)
	}
	wanted := make(map[string]bool, len(requested))
	for _, privilege := range requested {
		upper := strings.ToUpper(strings.TrimSpace(privilege))
		if !slices.Contains(allowed, upper) {
			return nil, fmt.Errorf("%w: %q is not a privilege that applies here; the choices are %s",
				ErrInvalidName, privilege, strings.Join(allowed, ", "))
		}
		wanted[upper] = true
	}
	out := make([]string, 0, len(wanted))
	for _, privilege := range allowed {
		if wanted[privilege] {
			out = append(out, privilege)
		}
	}
	return out, nil
}

// privilegeObject renders the ON clause's object for every target but the
// default ones.
func privilegeObject(quotedDatabase string, change PrivilegeChange) (string, error) {
	if change.Target == targetDatabase {
		return "DATABASE " + quotedDatabase, nil
	}
	quotedSchema, err := quoteName(change.Schema)
	if err != nil {
		return "", fmt.Errorf("schema: %w", err)
	}

	switch change.Target {
	case targetSchema:
		return "SCHEMA " + quotedSchema, nil
	case targetAllTables:
		return "ALL TABLES IN SCHEMA " + quotedSchema, nil
	case targetAllSequences:
		return "ALL SEQUENCES IN SCHEMA " + quotedSchema, nil
	}

	// "table" or "sequence": each object qualified by the schema, so a name that
	// also exists earlier on the search_path cannot be the one granted on.
	keyword := "TABLE "
	if change.Target == targetSequence {
		keyword = "SEQUENCE "
	}
	names := make([]string, 0, len(change.Objects))
	for _, object := range change.Objects {
		quoted, err := quoteName(object)
		if err != nil {
			return "", fmt.Errorf("object %q: %w", object, err)
		}
		names = append(names, quotedSchema+"."+quoted)
	}
	return keyword + strings.Join(names, ", "), nil
}

// defaultPrivilegesStatement renders ALTER DEFAULT PRIVILEGES.
//
// FOR ROLE is always written, never left to default to the current user. The
// current user is the panel's own account, and a default that covers only the
// tables the panel creates covers nothing an application creates through its own
// migrations — the statement would succeed and protect nothing.
func defaultPrivilegesStatement(change PrivilegeChange, clause, preposition, quotedRole string) (string, error) {
	quotedOwner, err := quoteIdentifier(change.Owner)
	if err != nil {
		return "", fmt.Errorf("owner: %w", err)
	}
	statement := "ALTER DEFAULT PRIVILEGES FOR ROLE " + quotedOwner
	if change.Schema != "" {
		quotedSchema, err := quoteName(change.Schema)
		if err != nil {
			return "", fmt.Errorf("schema: %w", err)
		}
		statement += " IN SCHEMA " + quotedSchema
	}
	objects := "TABLES"
	if change.Target == targetDefaultSequences {
		objects = "SEQUENCES"
	}
	return statement + " " + clause + " ON " + objects + " " + preposition + " " + quotedRole, nil
}

// orderPrivileges returns the privileges of a listing in the allowlist's order
// and drops any the panel does not show, so the API's answer is stable whatever
// order the catalog produced them in, and agrees with what it accepts.
func orderPrivileges(allowed, found []string) []string {
	out := []string{}
	for _, privilege := range allowed {
		if slices.Contains(found, privilege) {
			out = append(out, privilege)
		}
	}
	return out
}
