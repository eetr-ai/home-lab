"use server";

import { revalidatePath } from "next/cache";
import * as mongo from "@/lib/api/mongo";
import { withRead, withWrite } from "./_auth";
import type { ActionResult } from "@/lib/api/result";
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
} from "@/lib/api/types";

/** The MongoDB section's actions. See postgres.ts for why the whole section
 *  revalidates rather than one page. */
const SECTION = "/mongo";

export async function listDatabases(): Promise<ActionResult<MongoDatabase[]>> {
	return withRead(() => mongo.listDatabases());
}

export async function createDatabase(
	request: CreateMongoDatabase,
): Promise<ActionResult<MongoDatabase>> {
	return withWrite(async () => {
		const result = await mongo.createDatabase(request);
		if (result.ok) revalidatePath(SECTION, "layout");
		return result;
	});
}

export async function dropDatabase(name: string): Promise<ActionResult<void>> {
	return withWrite(async () => {
		const result = await mongo.dropDatabase(name);
		if (result.ok) revalidatePath(SECTION, "layout");
		return result;
	});
}

export async function listCollections(
	database: string,
): Promise<ActionResult<MongoCollection[]>> {
	return withRead(() => mongo.listCollections(database));
}

export async function createCollection(
	database: string,
	request: CreateMongoCollection,
): Promise<ActionResult<MongoCollection>> {
	return withWrite(async () => {
		const result = await mongo.createCollection(database, request);
		if (result.ok) revalidatePath(SECTION, "layout");
		return result;
	});
}

export async function dropCollection(
	database: string,
	name: string,
): Promise<ActionResult<void>> {
	return withWrite(async () => {
		const result = await mongo.dropCollection(database, name);
		if (result.ok) revalidatePath(SECTION, "layout");
		return result;
	});
}

export async function listUsers(database: string): Promise<ActionResult<MongoUser[]>> {
	return withRead(() => mongo.listUsers(database));
}

export async function createUser(
	database: string,
	request: CreateMongoUser,
): Promise<ActionResult<MongoUser>> {
	return withWrite(async () => {
		const result = await mongo.createUser(database, request);
		if (result.ok) revalidatePath(SECTION, "layout");
		return result;
	});
}

export async function dropUser(database: string, name: string): Promise<ActionResult<void>> {
	return withWrite(async () => {
		const result = await mongo.dropUser(database, name);
		if (result.ok) revalidatePath(SECTION, "layout");
		return result;
	});
}

export async function updateUser(
	database: string,
	name: string,
	request: UpdateMongoUser,
): Promise<ActionResult<MongoUser>> {
	const result = await withWrite(() => mongo.updateUser(database, name, request));
	if (result.ok) revalidatePath("/mongo", "layout");
	return result;
}

/**
 * Run a find.
 *
 * withRead, not withWrite. A find cannot write — aggregate and runCommand, which
 * can, are not offered at all — so gating this on write access would restrict a
 * read while protecting nothing.
 */
export async function find(
	database: string,
	request: FindRequest,
): Promise<ActionResult<FindResult>> {
	return withRead(() => mongo.find(database, request));
}
