package mongo

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// minPasswordLength is the shortest password this panel will set. The server
// listens on the LAN with no per-client allowlist and no TLS, so the password is
// the whole access boundary — the same reasoning as the PostgreSQL slice.
const minPasswordLength = 16

// protectedDatabases are MongoDB's own. admin holds every credential including
// the one this panel connects with, config holds sharding and session state, and
// local holds the oplog.
var protectedDatabases = []string{"admin", "config", "local"}

// forbiddenRoles are the ones that would make an account the equal of the
// superuser this panel connects as. Granting them through a web form would create
// an administrator nobody remembers appointing.
var forbiddenRoles = []string{
	"root",
	"__system",
	"userAdminAnyDatabase",
	"dbAdminAnyDatabase",
	"readWriteAnyDatabase",
	"readAnyDatabase",
	"restore",
	"backup",
}

// repository is the persistence this service needs. Declared here, where it is
// consumed, so the service can be tested without a MongoDB server.
type repository interface {
	ListDatabases(ctx context.Context) ([]Database, error)
	DropDatabase(ctx context.Context, name string) error
	DatabaseExists(ctx context.Context, name string) (bool, error)

	ListCollections(ctx context.Context, database string) ([]Collection, error)
	CreateCollection(ctx context.Context, database, name string) error
	DropCollection(ctx context.Context, database, name string) error
	CollectionExists(ctx context.Context, database, name string) (bool, error)

	ListUsers(ctx context.Context, database string) ([]User, error)
	CreateUser(ctx context.Context, database string, req CreateUserRequest) error
	DropUser(ctx context.Context, database, name string) error
	UserExists(ctx context.Context, database, name string) (bool, error)
	UpdateUser(ctx context.Context, database, name string, req UpdateUserRequest) error
	CurrentUser(ctx context.Context) (string, error)

	Find(ctx context.Context, database string, req FindRequest) (FindResult, error)
}

// Service manages the databases, collections, and users on the MongoDB server.
type Service struct {
	repo repository
}

// NewService builds the service.
func NewService(repo repository) *Service {
	return &Service{repo: repo}
}

// ListDatabases returns every database on the server.
func (s *Service) ListDatabases(ctx context.Context) ([]Database, error) {
	return s.repo.ListDatabases(ctx)
}

// CreateDatabase creates a database by creating its first collection.
func (s *Service) CreateDatabase(ctx context.Context, req CreateDatabaseRequest) (Database, error) {
	if err := validateDatabaseName(req.Name); err != nil {
		return Database{}, err
	}
	if err := validateCollectionName(req.Collection); err != nil {
		return Database{}, fmt.Errorf("collection: %w", err)
	}
	if slices.Contains(protectedDatabases, strings.ToLower(req.Name)) {
		return Database{}, fmt.Errorf("%w: %q is one of MongoDB's own databases", ErrProtected, req.Name)
	}

	exists, err := s.repo.DatabaseExists(ctx, req.Name)
	if err != nil {
		return Database{}, err
	}
	if exists {
		return Database{}, fmt.Errorf("%w: a database named %q", ErrAlreadyExists, req.Name)
	}

	if err := s.repo.CreateCollection(ctx, req.Name, req.Collection); err != nil {
		return Database{}, err
	}
	return Database{Name: req.Name, Empty: false}, nil
}

// DropDatabase removes a database and everything in it.
func (s *Service) DropDatabase(ctx context.Context, name string) error {
	if err := validateDatabaseName(name); err != nil {
		return err
	}
	if slices.Contains(protectedDatabases, strings.ToLower(name)) {
		return fmt.Errorf("%w: %q is one of MongoDB's own databases", ErrProtected, name)
	}

	exists, err := s.repo.DatabaseExists(ctx, name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: no database named %q", ErrNotFound, name)
	}
	return s.repo.DropDatabase(ctx, name)
}

// ListCollections returns the collections in one database.
func (s *Service) ListCollections(ctx context.Context, database string) ([]Collection, error) {
	if err := validateDatabaseName(database); err != nil {
		return nil, err
	}
	return s.repo.ListCollections(ctx, database)
}

// CreateCollection creates a collection in an existing database.
func (s *Service) CreateCollection(ctx context.Context, database string,
	req CreateCollectionRequest) (Collection, error) {
	if err := validateDatabaseName(database); err != nil {
		return Collection{}, err
	}
	if err := validateCollectionName(req.Name); err != nil {
		return Collection{}, err
	}
	if slices.Contains(protectedDatabases, strings.ToLower(database)) {
		return Collection{}, fmt.Errorf("%w: %q is one of MongoDB's own databases", ErrProtected, database)
	}

	exists, err := s.repo.CollectionExists(ctx, database, req.Name)
	if err != nil {
		return Collection{}, err
	}
	if exists {
		return Collection{}, fmt.Errorf("%w: a collection named %q in %q", ErrAlreadyExists, req.Name, database)
	}

	if err := s.repo.CreateCollection(ctx, database, req.Name); err != nil {
		return Collection{}, err
	}
	return Collection{Name: req.Name, Type: "collection"}, nil
}

// DropCollection removes a collection and its documents.
func (s *Service) DropCollection(ctx context.Context, database, name string) error {
	if err := validateDatabaseName(database); err != nil {
		return err
	}
	if err := validateCollectionName(name); err != nil {
		return err
	}
	if slices.Contains(protectedDatabases, strings.ToLower(database)) {
		return fmt.Errorf("%w: %q is one of MongoDB's own databases", ErrProtected, database)
	}

	exists, err := s.repo.CollectionExists(ctx, database, name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: no collection named %q in %q", ErrNotFound, name, database)
	}
	return s.repo.DropCollection(ctx, database, name)
}

// ListUsers returns the users defined in one database.
func (s *Service) ListUsers(ctx context.Context, database string) ([]User, error) {
	if err := validateDatabaseName(database); err != nil {
		return nil, err
	}
	return s.repo.ListUsers(ctx, database)
}

// CreateUser creates a user in one database.
//
// MongoDB scopes a user to the database it is created in, and that database is
// where it authenticates — so the caller chooses it rather than everything
// landing in admin, which is where the superuser lives.
func (s *Service) CreateUser(ctx context.Context, database string, req CreateUserRequest) (User, error) {
	if err := validateDatabaseName(database); err != nil {
		return User{}, err
	}
	if err := validateUserName(req.Name); err != nil {
		return User{}, err
	}
	if len(req.Password) < minPasswordLength {
		return User{}, fmt.Errorf("%w: a password of at least %d characters is required",
			ErrWeakPassword, minPasswordLength)
	}
	if err := validateRoles(req.Roles); err != nil {
		return User{}, err
	}

	exists, err := s.repo.UserExists(ctx, database, req.Name)
	if err != nil {
		return User{}, err
	}
	if exists {
		return User{}, fmt.Errorf("%w: a user named %q in %q", ErrAlreadyExists, req.Name, database)
	}

	if err := s.repo.CreateUser(ctx, database, req); err != nil {
		return User{}, err
	}
	return User{Name: req.Name, Database: database, Roles: req.Roles}, nil
}

// DropUser removes a user.
func (s *Service) DropUser(ctx context.Context, database, name string) error {
	if err := validateDatabaseName(database); err != nil {
		return err
	}
	if err := validateUserName(name); err != nil {
		return err
	}

	// The panel must not remove the account it connects as. MongoDB would allow
	// it, and an admin panel that locked itself out of the server it administers
	// is worth checking for rather than hoping about.
	current, err := s.repo.CurrentUser(ctx)
	if err != nil {
		return err
	}
	if strings.EqualFold(current, name) {
		return fmt.Errorf("%w: %q is the account this panel connects as", ErrProtected, name)
	}

	exists, err := s.repo.UserExists(ctx, database, name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: no user named %q in %q", ErrNotFound, name, database)
	}
	return s.repo.DropUser(ctx, database, name)
}

// validateRoles rejects a grant this panel will not make.
func validateRoles(roles []Role) error {
	if len(roles) == 0 {
		// A user with no roles can authenticate and do nothing, which is almost
		// never what was meant and is confusing to diagnose.
		return fmt.Errorf("%w: at least one role is required", ErrInvalidRole)
	}
	for _, role := range roles {
		if err := validateUserName(role.Name); err != nil {
			return fmt.Errorf("%w: %q is not a role name", ErrInvalidRole, role.Name)
		}
		if role.Database != "" {
			if err := validateDatabaseName(role.Database); err != nil {
				return fmt.Errorf("role %q: %w", role.Name, err)
			}
		}
		if slices.Contains(forbiddenRoles, role.Name) {
			return fmt.Errorf("%w: %q would make the account an administrator of the whole server",
				ErrInvalidRole, role.Name)
		}
	}
	return nil
}

// UpdateUser replaces a user's roles and, when one is given, its password.
//
// The account the panel connects as is refused, for the same reason it cannot be
// dropped: revoking its own roles through a form would lock the panel out of the
// server it is being used to administer.
func (s *Service) UpdateUser(
	ctx context.Context, database, name string, req UpdateUserRequest,
) (User, error) {
	if err := validateDatabaseName(database); err != nil {
		return User{}, err
	}
	if err := validateUserName(name); err != nil {
		return User{}, err
	}
	if req.Password != "" && len(req.Password) < minPasswordLength {
		return User{}, fmt.Errorf("%w: a password must be at least %d characters",
			ErrWeakPassword, minPasswordLength)
	}
	if err := validateRoles(req.Roles); err != nil {
		return User{}, err
	}

	current, err := s.repo.CurrentUser(ctx)
	if err != nil {
		return User{}, err
	}
	// EqualFold, matching the drop path a few functions up. The same invariant
	// spelled two ways in one file is how a name differing only in case ends up
	// refused by one and accepted by the other.
	if strings.EqualFold(current, name) {
		return User{}, fmt.Errorf(
			"%w: %q is the account the panel connects as; changing it here could lock the panel out",
			ErrProtected, name)
	}

	exists, err := s.repo.UserExists(ctx, database, name)
	if err != nil {
		return User{}, err
	}
	if !exists {
		return User{}, fmt.Errorf("%w: no user named %q in %q", ErrNotFound, name, database)
	}

	if err := s.repo.UpdateUser(ctx, database, name, req); err != nil {
		return User{}, err
	}
	return User{Name: name, Database: database, Roles: req.Roles}, nil
}

// Find returns documents from one collection.
//
// A find and nothing else — no aggregate, which can write through $out and
// $merge, and no runCommand, which is the whole server. Both are deliberate
// omissions rather than things not got to yet.
//
// The filter is checked for the operators that run server-side JavaScript, and
// this check is the whole of that boundary: verified against MongoDB 7, the
// server permits $where, $function, and a $where nested inside an $and on a root
// connection without complaint. Nothing below this refuses them.
//
// What they actually are, having looked rather than assumed: JavaScript in the
// server's own sandboxed engine, with no require, process, fs, or db in scope —
// so not a route to the host, unlike the PostgreSQL side's COPY TO PROGRAM. What
// they are is a way to force a full collection scan and to occupy a core until
// the query deadline: an unbounded loop in a $where ran for the full maxTimeMS
// before being killed. That is worth refusing from a panel button, and worth
// refusing honestly rather than by overstating it.
//
// The check is a closed list of operator names read from a typed document, not a
// pattern match over text, which is why it can be relied on at all.
func (s *Service) Find(ctx context.Context, database string, req FindRequest) (FindResult, error) {
	if err := validateDatabaseName(database); err != nil {
		return FindResult{}, err
	}
	if err := validateCollectionName(req.Collection); err != nil {
		return FindResult{}, err
	}
	if err := refuseJavaScript(req.Filter); err != nil {
		return FindResult{}, err
	}
	if err := refuseJavaScript(req.Sort); err != nil {
		return FindResult{}, err
	}
	if err := refuseJavaScript(req.Projection); err != nil {
		return FindResult{}, err
	}
	return s.repo.Find(ctx, database, req)
}

// javaScriptOperators are the query operators that execute code on the server.
//
// $where and $function both run JavaScript with the connection's privileges;
// $accumulator does the same inside an aggregation and is listed because a filter
// document is not the only place one can appear. $expr is deliberately absent —
// it evaluates aggregation expressions, not JavaScript, and is a legitimate part
// of an ordinary query.
var javaScriptOperators = []string{"$where", "$function", "$accumulator"}

// refuseJavaScript walks a query document for operators that execute code.
//
// Recursive, because these nest: a $where inside an $and inside an $or is the
// same instruction to the server as one at the top level, and a check that only
// looked at the outermost keys would be a check in name only.
func refuseJavaScript(document map[string]any) error {
	for key, value := range document {
		if slices.Contains(javaScriptOperators, strings.ToLower(key)) {
			return fmt.Errorf("%w: %s runs JavaScript on the server and is not permitted here",
				ErrInvalidQuery, key)
		}
		if err := refuseJavaScriptIn(value); err != nil {
			return err
		}
	}
	return nil
}

// refuseJavaScriptIn descends into whatever a value turns out to be.
//
// A JSON body decodes into map[string]any and []any, so those two are the whole
// of what can hold a nested operator.
func refuseJavaScriptIn(value any) error {
	switch nested := value.(type) {
	case map[string]any:
		return refuseJavaScript(nested)
	case []any:
		for _, item := range nested {
			if err := refuseJavaScriptIn(item); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}
