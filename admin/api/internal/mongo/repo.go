package mongo

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	driver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Repository is the MongoDB-backed persistence for this slice.
//
// Every operation here is a database command sent as BSON with typed fields, so
// a name travels as a value and is never parsed as syntax. That is why the name
// rules in names.go are about what MongoDB can store rather than about escaping.
type Repository struct {
	client *driver.Client
}

// NewRepository builds a client. It does not connect: the driver dials lazily, so
// an unreachable server costs a failed request rather than a process that will
// not start.
func NewRepository(uri string) (*Repository, error) {
	client, err := driver.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("configure the MongoDB client: %w", err)
	}
	return &Repository{client: client}, nil
}

// Close disconnects the client.
func (r *Repository) Close(ctx context.Context) {
	_ = r.client.Disconnect(ctx)
}

// ListDatabases returns every database on the server.
func (r *Repository) ListDatabases(ctx context.Context) ([]Database, error) {
	result, err := r.client.ListDatabases(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}

	databases := make([]Database, 0, len(result.Databases))
	for _, specification := range result.Databases {
		databases = append(databases, Database{
			Name:      specification.Name,
			SizeBytes: specification.SizeOnDisk,
			Empty:     specification.Empty,
		})
	}
	return databases, nil
}

// DatabaseExists reports whether a database is present.
//
// A database MongoDB lists is one that holds something: there is no catalog entry
// for an empty one, because creating a database and putting the first thing in it
// are the same act.
func (r *Repository) DatabaseExists(ctx context.Context, name string) (bool, error) {
	names, err := r.client.ListDatabaseNames(ctx, bson.D{{Key: "name", Value: name}})
	if err != nil {
		return false, fmt.Errorf("check for database %q: %w", name, err)
	}
	return len(names) > 0, nil
}

// DropDatabase removes a database and everything in it.
func (r *Repository) DropDatabase(ctx context.Context, name string) error {
	if err := r.client.Database(name).Drop(ctx); err != nil {
		return fmt.Errorf("drop database %q: %w", name, err)
	}
	return nil
}

// ListCollections returns the collections in one database.
func (r *Repository) ListCollections(ctx context.Context, database string) ([]Collection, error) {
	specifications, err := r.client.Database(database).ListCollectionSpecifications(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("list collections in %q: %w", database, err)
	}

	collections := make([]Collection, 0, len(specifications))
	for _, specification := range specifications {
		collections = append(collections, Collection{Name: specification.Name, Type: specification.Type})
	}
	return collections, nil
}

// CollectionExists reports whether a collection is present.
func (r *Repository) CollectionExists(ctx context.Context, database, name string) (bool, error) {
	names, err := r.client.Database(database).ListCollectionNames(ctx, bson.D{{Key: "name", Value: name}})
	if err != nil {
		return false, fmt.Errorf("check for collection %q in %q: %w", name, database, err)
	}
	return len(names) > 0, nil
}

// CreateCollection creates a collection, bringing its database into existence if
// this is the first thing in it.
func (r *Repository) CreateCollection(ctx context.Context, database, name string) error {
	if err := r.client.Database(database).CreateCollection(ctx, name); err != nil {
		return fmt.Errorf("create collection %q in %q: %w", name, database, err)
	}
	return nil
}

// DropCollection removes a collection and its documents.
func (r *Repository) DropCollection(ctx context.Context, database, name string) error {
	if err := r.client.Database(database).Collection(name).Drop(ctx); err != nil {
		return fmt.Errorf("drop collection %q in %q: %w", name, database, err)
	}
	return nil
}

// usersInfoResult is the shape of the usersInfo command's reply.
type usersInfoResult struct {
	Users []struct {
		User  string `bson:"user"`
		DB    string `bson:"db"`
		Roles []struct {
			Role string `bson:"role"`
			DB   string `bson:"db"`
		} `bson:"roles"`
	} `bson:"users"`
}

// ListUsers returns the users defined in one database.
func (r *Repository) ListUsers(ctx context.Context, database string) ([]User, error) {
	var result usersInfoResult
	err := r.client.Database(database).
		RunCommand(ctx, bson.D{{Key: "usersInfo", Value: 1}}).
		Decode(&result)
	if err != nil {
		return nil, fmt.Errorf("list users in %q: %w", database, err)
	}

	users := make([]User, 0, len(result.Users))
	for _, entry := range result.Users {
		roles := make([]Role, 0, len(entry.Roles))
		for _, role := range entry.Roles {
			roles = append(roles, Role{Name: role.Role, Database: role.DB})
		}
		users = append(users, User{Name: entry.User, Database: entry.DB, Roles: roles})
	}
	return users, nil
}

// UserExists reports whether a user is defined in one database.
func (r *Repository) UserExists(ctx context.Context, database, name string) (bool, error) {
	var result usersInfoResult
	err := r.client.Database(database).
		RunCommand(ctx, bson.D{{Key: "usersInfo", Value: bson.D{
			{Key: "user", Value: name},
			{Key: "db", Value: database},
		}}}).
		Decode(&result)
	if err != nil {
		return false, fmt.Errorf("check for user %q in %q: %w", name, database, err)
	}
	return len(result.Users) > 0, nil
}

// CreateUser creates a user in one database.
func (r *Repository) CreateUser(ctx context.Context, database string, req CreateUserRequest) error {
	roles := make(bson.A, 0, len(req.Roles))
	for _, role := range req.Roles {
		on := role.Database
		if on == "" {
			on = database
		}
		roles = append(roles, bson.D{{Key: "role", Value: role.Name}, {Key: "db", Value: on}})
	}

	// The password travels as a BSON field rather than inside a command string,
	// and the driver negotiates SCRAM-SHA-256 with the server, so it is not
	// stored as given. Nothing here logs the command.
	command := bson.D{
		{Key: "createUser", Value: req.Name},
		{Key: "pwd", Value: req.Password},
		{Key: "roles", Value: roles},
	}
	if err := r.client.Database(database).RunCommand(ctx, command).Err(); err != nil {
		return fmt.Errorf("create user %q in %q: %w", req.Name, database, err)
	}
	return nil
}

// DropUser removes a user.
func (r *Repository) DropUser(ctx context.Context, database, name string) error {
	err := r.client.Database(database).
		RunCommand(ctx, bson.D{{Key: "dropUser", Value: name}}).Err()
	if err != nil {
		return fmt.Errorf("drop user %q in %q: %w", name, database, err)
	}
	return nil
}

// connectionStatusResult is the part of connectionStatus this slice reads.
type connectionStatusResult struct {
	AuthInfo struct {
		AuthenticatedUsers []struct {
			User string `bson:"user"`
		} `bson:"authenticatedUsers"`
	} `bson:"authInfo"`
}

// CurrentUser reports the account the panel is authenticated as.
func (r *Repository) CurrentUser(ctx context.Context) (string, error) {
	var result connectionStatusResult
	err := r.client.Database("admin").
		RunCommand(ctx, bson.D{{Key: "connectionStatus", Value: 1}}).
		Decode(&result)
	if err != nil {
		return "", fmt.Errorf("read the connection status: %w", err)
	}
	if len(result.AuthInfo.AuthenticatedUsers) == 0 {
		// Unauthenticated is not a state this panel can administer from, and
		// reporting an empty name would make the self-deletion guard pass.
		return "", errors.New("the MongoDB connection is not authenticated")
	}
	return result.AuthInfo.AuthenticatedUsers[0].User, nil
}
