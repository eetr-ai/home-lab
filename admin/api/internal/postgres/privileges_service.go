package postgres

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// ListPrivileges returns what one role holds in one database.
func (s *Service) ListPrivileges(ctx context.Context, database, role string) (RolePrivileges, error) {
	if err := validateName(database); err != nil {
		return RolePrivileges{}, err
	}
	if err := validateName(role); err != nil {
		return RolePrivileges{}, fmt.Errorf("role: %w", err)
	}

	existing, err := s.privilegeSubject(ctx, database, role)
	if err != nil {
		return RolePrivileges{}, err
	}

	privileges, err := s.repo.ListPrivileges(ctx, database, role)
	if err != nil {
		return RolePrivileges{}, err
	}
	privileges.IsSuperuser = existing.IsSuperuser
	return privileges, nil
}

// ChangePrivileges grants and revokes a set of privileges for one role in one
// database, as one transaction, and returns the statements it ran and where they
// left the role.
//
// Three kinds of role are refused, each for a reason the rest of this slice
// already gives:
//
//   - PostgreSQL's own (pg_*). Their privileges are the server's to define, and
//     granting a predefined role rights on application tables hands them to
//     every member of it, which is nobody the operator is looking at.
//   - The role the panel connects as. It is a superuser, so a grant to it changes
//     nothing, and the panel does not alter itself on any path.
//   - Any other superuser, for the same first reason: a superuser bypasses every
//     privilege check, so a GRANT would succeed and mean nothing. Refusing says
//     so instead of letting the page show a change that has no effect.
//
// PostgreSQL's own databases are refused as well. template1 is copied into every
// database created after it, so a grant there would quietly appear everywhere.
func (s *Service) ChangePrivileges(
	ctx context.Context, database, role string, req ChangePrivilegesRequest,
) (ChangePrivilegesResult, error) {
	// Built first: it is the whole of the input validation, and it needs no
	// server, so a malformed request is refused before anything is asked.
	statements, err := privilegeStatements(database, role, req.Changes)
	if err != nil {
		return ChangePrivilegesResult{}, err
	}
	if strings.HasPrefix(strings.ToLower(role), reservedPrefix) {
		return ChangePrivilegesResult{}, fmt.Errorf("%w: %q is one of PostgreSQL's own roles", ErrProtected, role)
	}
	if slices.Contains(protectedDatabases, strings.ToLower(database)) {
		return ChangePrivilegesResult{}, fmt.Errorf("%w: %q is one of PostgreSQL's own databases", ErrProtected, database)
	}

	current, err := s.repo.CurrentRole(ctx)
	if err != nil {
		return ChangePrivilegesResult{}, err
	}
	if strings.EqualFold(current, role) {
		return ChangePrivilegesResult{}, fmt.Errorf("%w: %q is the account this panel connects as", ErrProtected, role)
	}

	existing, err := s.privilegeSubject(ctx, database, role)
	if err != nil {
		return ChangePrivilegesResult{}, err
	}
	if existing.IsSuperuser {
		return ChangePrivilegesResult{}, fmt.Errorf(
			"%w: %q is a superuser, which bypasses every privilege check; granting to it would change nothing",
			ErrProtected, role)
	}
	if err := s.checkDefaultOwners(ctx, req.Changes); err != nil {
		return ChangePrivilegesResult{}, err
	}

	// The state comes back from inside the transaction that applied the change,
	// so a committed change is never reported as a failure because a second read
	// afterwards did not succeed.
	privileges, err := s.repo.ApplyPrivileges(ctx, database, role, req.Changes, statements)
	if err != nil {
		return ChangePrivilegesResult{}, err
	}
	return ChangePrivilegesResult{Statements: statements, Privileges: privileges}, nil
}

// privilegeSubject checks that the database and role both exist, and returns the
// role. Checked here rather than left to the server, because PostgreSQL's message
// for a missing role in a GRANT does not say which of the request's names it was.
func (s *Service) privilegeSubject(ctx context.Context, database, role string) (Role, error) {
	exists, err := s.repo.DatabaseExists(ctx, database)
	if err != nil {
		return Role{}, err
	}
	if !exists {
		return Role{}, fmt.Errorf("%w: no database named %q", ErrNotFound, database)
	}

	roles, err := s.repo.ListRoles(ctx)
	if err != nil {
		return Role{}, err
	}
	existing, found := findRole(roles, role)
	if !found {
		return Role{}, fmt.Errorf("%w: no role named %q", ErrNotFound, role)
	}
	return existing, nil
}

// checkDefaultOwners requires every default-privileges owner to exist, for the
// same reason privilegeSubject checks the role.
func (s *Service) checkDefaultOwners(ctx context.Context, changes []PrivilegeChange) error {
	checked := map[string]bool{}
	for _, change := range changes {
		if change.Owner == "" || checked[change.Owner] {
			continue
		}
		exists, err := s.repo.RoleExists(ctx, change.Owner)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: no role named %q to own future objects", ErrNotFound, change.Owner)
		}
		checked[change.Owner] = true
	}
	return nil
}
