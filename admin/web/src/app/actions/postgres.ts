"use server";

import { revalidatePath } from "next/cache";
import * as postgres from "@/lib/api/postgres";
import { withRead, withWrite } from "./_auth";
import type { ActionResult } from "@/lib/api/result";
import type {
	CreatePostgresDatabase,
	CreatePostgresRole,
	PostgresDatabase,
	PostgresExtension,
	PostgresRole,
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
