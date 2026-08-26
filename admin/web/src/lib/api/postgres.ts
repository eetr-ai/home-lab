import { call, seg } from "./http";
import type { ActionResult } from "./result";
import type {
	CreatePostgresDatabase,
	CreatePostgresRole,
	PostgresDatabase,
	PostgresExtension,
	PostgresRole,
} from "./types";

/**
 * The PostgreSQL operations, named for what they do rather than for the verb and
 * path that carry them out. Mirrors admin/api/internal/postgres, so the vertical
 * seam runs from the browser to the database.
 */

export function listDatabases(): Promise<ActionResult<PostgresDatabase[]>> {
	return call<PostgresDatabase[]>("GET", "/api/postgres/databases");
}

export function createDatabase(
	request: CreatePostgresDatabase,
): Promise<ActionResult<PostgresDatabase>> {
	return call<PostgresDatabase>("POST", "/api/postgres/databases", request);
}

export function dropDatabase(name: string): Promise<ActionResult<void>> {
	return call<void>("DELETE", `/api/postgres/databases/${seg(name)}`);
}

export function listRoles(): Promise<ActionResult<PostgresRole[]>> {
	return call<PostgresRole[]>("GET", "/api/postgres/roles");
}

export function createRole(request: CreatePostgresRole): Promise<ActionResult<PostgresRole>> {
	return call<PostgresRole>("POST", "/api/postgres/roles", request);
}

export function dropRole(name: string): Promise<ActionResult<void>> {
	return call<void>("DELETE", `/api/postgres/roles/${seg(name)}`);
}

/** Extensions are per-database, so listing them names one. */
export function listExtensions(database: string): Promise<ActionResult<PostgresExtension[]>> {
	return call<PostgresExtension[]>("GET", `/api/postgres/databases/${seg(database)}/extensions`);
}

export function installExtension(
	database: string,
	name: string,
): Promise<ActionResult<PostgresExtension>> {
	return call<PostgresExtension>("POST", `/api/postgres/databases/${seg(database)}/extensions`, {
		name,
	});
}
