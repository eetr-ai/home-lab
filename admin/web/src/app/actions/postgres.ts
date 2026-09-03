"use server";

import { revalidatePath } from "next/cache";
import * as postgres from "@/lib/api/postgres";
import { withRead, withWrite } from "./_auth";
import type { ActionResult } from "@/lib/api/result";
import type {
	BrowseRequest,
	BrowseResult,
	CreatePostgresDatabase,
	CreatePostgresRole,
	PostgresDatabase,
	PostgresExtension,
	PostgresRelation,
	PostgresRole,
	QueryResult,
	UpdatePostgresDatabase,
	UpdatePostgresRole,
} from "@/lib/api/types";

/**
 * The PostgreSQL section's actions. Each authorizes and then delegates — no
 * business logic here, and none in the pages either: it lives in the API, where
 * the agent reaches it too.
 *
 * Mutations revalidate the section rather than one page. Creating a role changes
 * what the databases tab can show as an owner, and dropping a database changes
 * which extensions can be listed, so refreshing only the page that acted would
 * leave a sibling tab stale.
 */
const SECTION = "/postgres";

export async function listDatabases(): Promise<ActionResult<PostgresDatabase[]>> {
	return withRead(() => postgres.listDatabases());
}

export async function createDatabase(
	request: CreatePostgresDatabase,
): Promise<ActionResult<PostgresDatabase>> {
	return withWrite(async () => {
		const result = await postgres.createDatabase(request);
		if (result.ok) revalidatePath(SECTION, "layout");
		return result;
	});
}

export async function dropDatabase(name: string): Promise<ActionResult<void>> {
	return withWrite(async () => {
		const result = await postgres.dropDatabase(name);
		if (result.ok) revalidatePath(SECTION, "layout");
		return result;
	});
}

export async function listRoles(): Promise<ActionResult<PostgresRole[]>> {
	return withRead(() => postgres.listRoles());
}

export async function createRole(
	request: CreatePostgresRole,
): Promise<ActionResult<PostgresRole>> {
	return withWrite(async () => {
		const result = await postgres.createRole(request);
		if (result.ok) revalidatePath(SECTION, "layout");
		return result;
	});
}

export async function dropRole(name: string): Promise<ActionResult<void>> {
	return withWrite(async () => {
		const result = await postgres.dropRole(name);
		if (result.ok) revalidatePath(SECTION, "layout");
		return result;
	});
}

export async function listExtensions(
	database: string,
): Promise<ActionResult<PostgresExtension[]>> {
	return withRead(() => postgres.listExtensions(database));
}

export async function installExtension(
	database: string,
	name: string,
): Promise<ActionResult<PostgresExtension>> {
	return withWrite(async () => {
		const result = await postgres.installExtension(database, name);
		if (result.ok) revalidatePath(SECTION, "layout");
		return result;
	});
}

export async function updateRole(
	name: string,
	request: UpdatePostgresRole,
): Promise<ActionResult<PostgresRole>> {
	const result = await withWrite(() => postgres.updateRole(name, request));
	if (result.ok) revalidatePath("/postgres", "layout");
	return result;
}

export async function updateDatabase(
	name: string,
	request: UpdatePostgresDatabase,
): Promise<ActionResult<void>> {
	const result = await withWrite(() => postgres.updateDatabase(name, request));
	if (result.ok) revalidatePath("/postgres", "layout");
	return result;
}

/**
 * Run a read-only statement.
 *
 * withRead, not withWrite: this is a read, and the server is what guarantees it —
 * the statement runs in a READ ONLY transaction that is always rolled back.
 * Requiring write access for a SELECT would gate the wrong thing while doing
 * nothing about what it was meant to protect.
 */
export async function runQuery(
	database: string,
	sql: string,
): Promise<ActionResult<QueryResult>> {
	return withRead(() => postgres.runQuery(database, sql));
}

/** The schema tree's tables and views. A read, like listing anything else. */
export async function listTables(
	database: string,
): Promise<ActionResult<PostgresRelation[]>> {
	return withRead(() => postgres.listTables(database));
}

/**
 * One page of a table. withRead like runQuery: browsing is a read, and the same
 * READ ONLY transaction on the server is what guarantees it.
 */
export async function browseTable(
	database: string,
	request: BrowseRequest,
): Promise<ActionResult<BrowseResult>> {
	return withRead(() => postgres.browseTable(database, request));
}
