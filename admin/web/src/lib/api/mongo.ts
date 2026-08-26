import { call, seg } from "./http";
import type { ActionResult } from "./result";
import type {
	CreateMongoCollection,
	CreateMongoDatabase,
	CreateMongoUser,
	MongoCollection,
	MongoDatabase,
	MongoUser,
	FindRequest,
	FindResult,
	UpdateMongoUser,
} from "./types";

/**
 * The MongoDB operations. Mirrors admin/api/internal/mongo.
 *
 * Note that creating a database takes a collection name: MongoDB has no standalone
 * create-database, and one with nothing in it does not persist.
 */

export function listDatabases(): Promise<ActionResult<MongoDatabase[]>> {
	return call<MongoDatabase[]>("GET", "/api/mongo/databases");
}

export function createDatabase(request: CreateMongoDatabase): Promise<ActionResult<MongoDatabase>> {
	return call<MongoDatabase>("POST", "/api/mongo/databases", request);
}

export function dropDatabase(name: string): Promise<ActionResult<void>> {
	return call<void>("DELETE", `/api/mongo/databases/${seg(name)}`);
}

export function listCollections(database: string): Promise<ActionResult<MongoCollection[]>> {
	return call<MongoCollection[]>("GET", `/api/mongo/databases/${seg(database)}/collections`);
}

export function createCollection(
	database: string,
	request: CreateMongoCollection,
): Promise<ActionResult<MongoCollection>> {
	return call<MongoCollection>(
		"POST",
		`/api/mongo/databases/${seg(database)}/collections`,
		request,
	);
}

export function dropCollection(database: string, name: string): Promise<ActionResult<void>> {
	return call<void>("DELETE", `/api/mongo/databases/${seg(database)}/collections/${seg(name)}`);
}

/** Users are scoped to the database they authenticate against. */
export function listUsers(database: string): Promise<ActionResult<MongoUser[]>> {
	return call<MongoUser[]>("GET", `/api/mongo/databases/${seg(database)}/users`);
}

export function createUser(
	database: string,
	request: CreateMongoUser,
): Promise<ActionResult<MongoUser>> {
	return call<MongoUser>("POST", `/api/mongo/databases/${seg(database)}/users`, request);
}

export function dropUser(database: string, name: string): Promise<ActionResult<void>> {
	return call<void>("DELETE", `/api/mongo/databases/${seg(database)}/users/${seg(name)}`);
}

export function updateUser(
	database: string,
	name: string,
	request: UpdateMongoUser,
): Promise<ActionResult<MongoUser>> {
	return call<MongoUser>(
		"PUT",
		`/api/mongo/databases/${seg(database)}/users/${seg(name)}`,
		request,
	);
}

/** POST because the query goes in the body. It is a read; only find is offered. */
export function find(database: string, request: FindRequest): Promise<ActionResult<FindResult>> {
	return call<FindResult>("POST", `/api/mongo/databases/${seg(database)}/find`, request);
}
