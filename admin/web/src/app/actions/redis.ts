"use server";

import { revalidatePath } from "next/cache";
import * as redis from "@/lib/api/redis";
import * as kube from "@/lib/api/kube";
import { withRead, withWrite } from "./_auth";
import type { ActionResult } from "@/lib/api/result";
import type { Namespace } from "@/lib/api/types";
import type {
	CreateRedisUser,
	RedisKeyDetail,
	RedisKeyPage,
	RedisOverview,
	RedisText,
	RedisUser,
	RedisUserList,
	UpdateRedisUser,
} from "@/lib/api/redis-types";

/** The Redis section's actions. See postgres.ts for why the whole section
 *  revalidates rather than one page. */
const SECTION = "/redis";

export async function readOverview(): Promise<ActionResult<RedisOverview>> {
	return withRead(() => redis.readOverview());
}

export async function scanKeys(pattern: string, cursor: string): Promise<ActionResult<RedisKeyPage>> {
	return withRead(() => redis.scanKeys(pattern, cursor));
}

export async function readKey(key: RedisText): Promise<ActionResult<RedisKeyDetail>> {
	return withRead(() => redis.readKey(key));
}

/** Revalidates the section like every other mutation, so the overview's counts
 *  are not stale. The key browser keeps its place through it: it is keyed by the
 *  search, which a revalidation does not change (lib/redis/search.ts). */
export async function deleteKey(key: RedisText): Promise<ActionResult<void>> {
	return withWrite(async () => {
		const result = await redis.deleteKey(key);
		if (result.ok) revalidatePath(SECTION, "layout");
		return result;
	});
}

export async function listUsers(): Promise<ActionResult<RedisUserList>> {
	return withRead(() => redis.listUsers());
}

export async function createUser(request: CreateRedisUser): Promise<ActionResult<RedisUser>> {
	return withWrite(async () => {
		const result = await redis.createUser(request);
		if (result.ok) revalidatePath(SECTION, "layout");
		return result;
	});
}

export async function updateUser(name: string, request: UpdateRedisUser): Promise<ActionResult<RedisUser>> {
	return withWrite(async () => {
		const result = await redis.updateUser(name, request);
		if (result.ok) revalidatePath(SECTION, "layout");
		return result;
	});
}

export async function deleteUser(name: string): Promise<ActionResult<void>> {
	return withWrite(async () => {
		const result = await redis.deleteUser(name);
		if (result.ok) revalidatePath(SECTION, "layout");
		return result;
	});
}

/**
 * The namespace label, which is the cluster slice's write but this section's
 * question. Both sections revalidate: the Kubernetes namespaces page shows labels
 * too, and leaving it stale is the sibling-tab problem postgres.ts describes.
 */
export async function setRedisAccess(namespace: string, granted: boolean): Promise<ActionResult<Namespace>> {
	return withWrite(async () => {
		const result = granted ? await kube.grantRedisAccess(namespace) : await kube.revokeRedisAccess(namespace);
		if (result.ok) {
			revalidatePath(SECTION, "layout");
			revalidatePath("/kubernetes", "layout");
		}
		return result;
	});
}
