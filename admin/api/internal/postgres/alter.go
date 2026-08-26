package postgres

import (
	"context"
	"fmt"
	"strconv"
)

// AlterRole sets a role's flags, connection limit, and optionally its password.
func (r *Repository) AlterRole(ctx context.Context, name string, req UpdateRoleRequest) error {
	statement, err := alterRoleStatement(name, req)
	if err != nil {
		return err
	}
	// The statement may carry a SCRAM verifier as a literal, so it must never
	// reach a log. pgx does not log statements by default; nothing here adds one.
	if _, err := r.pool.Exec(ctx, statement); err != nil {
		return fmt.Errorf("alter role %q: %w", name, err)
	}
	return nil
}

// alterRoleStatement builds the ALTER ROLE, kept separate so it can be reasoned
// about — and, being pure, tested — without a server.
//
// Every attribute is written explicitly, including the negative form. ALTER ROLE
// leaves an unmentioned attribute alone, so omitting NOLOGIN when CanLogin is
// false would silently fail to revoke it — and the panel would then display a
// role as unable to log in while it still could.
func alterRoleStatement(name string, req UpdateRoleRequest) (string, error) {
	quotedName, err := quoteIdentifier(name)
	if err != nil {
		return "", err
	}

	statement := "ALTER ROLE " + quotedName + " WITH NOSUPERUSER"
	statement += booleanAttribute(req.CanLogin, " LOGIN", " NOLOGIN")
	statement += booleanAttribute(req.CanCreateDatabase, " CREATEDB", " NOCREATEDB")
	statement += booleanAttribute(req.CanCreateRole, " CREATEROLE", " NOCREATEROLE")
	// strconv rather than interpolation: the value is an int by the time it gets
	// here, so this cannot carry syntax, but writing it through the same kind of
	// deliberate conversion as everything else keeps the rule uniform.
	statement += " CONNECTION LIMIT " + strconv.Itoa(req.ConnectionLimit)

	if req.Password == "" {
		return statement, nil
	}

	// The verifier, never the password — the same reason as on create: plaintext
	// in a statement is visible in pg_stat_activity while it runs and lands in the
	// server log whenever log_statement is on.
	verifier, err := scramVerifier(req.Password)
	if err != nil {
		return "", err
	}
	quotedVerifier, err := quoteLiteral(verifier)
	if err != nil {
		return "", err
	}
	return statement + " PASSWORD " + quotedVerifier, nil
}

func booleanAttribute(enabled bool, whenTrue, whenFalse string) string {
	if enabled {
		return whenTrue
	}
	return whenFalse
}

// AlterDatabaseOwner reassigns a database to another role.
func (r *Repository) AlterDatabaseOwner(ctx context.Context, name, owner string) error {
	quotedName, err := quoteIdentifier(name)
	if err != nil {
		return err
	}
	quotedOwner, err := quoteIdentifier(owner)
	if err != nil {
		return err
	}

	if _, err := r.pool.Exec(ctx, "ALTER DATABASE "+quotedName+" OWNER TO "+quotedOwner); err != nil {
		return fmt.Errorf("change the owner of %q: %w", name, err)
	}
	return nil
}
