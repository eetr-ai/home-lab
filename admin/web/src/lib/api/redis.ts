import { call, seg } from "./http";
import type { ActionResult } from "./result";
import type {
	CreateRedisUser,
	RedisKeyDetail,
	RedisKeyPage,
	RedisOverview,
	RedisText,
	RedisUser,
	RedisUserList,
	UpdateRedisUser,
} from "./redis-types";
import { keySegment } from "@/lib/redis/text";

/**
 * The shared Redis's operations. Mirrors admin/api/internal/redis.
 *
 * There is no flush, on purpose: the server holds the token-refresh locks that
 * keep operators signed in. One key at a time is the most the panel removes.
 */

export function readOverview(): Promise<ActionResult<RedisOverview>> {
	return call<RedisOverview>("GET", "/api/redis/overview");
}

/** One SCAN step. Pass back `nextCursor` to continue; empty starts over. */
export function scanKeys(pattern: string, cursor: string): Promise<ActionResult<RedisKeyPage>> {
	const query = new URLSearchParams();
	if (pattern) query.set("pattern", pattern);
	if (cursor) query.set("cursor", cursor);
	const suffix = query.size > 0 ? `?${query}` : "";
	return call<RedisKeyPage>("GET", `/api/redis/keys${suffix}`);
}

export function readKey(key: RedisText): Promise<ActionResult<RedisKeyDetail>> {
	return call<RedisKeyDetail>("GET", `/api/redis/keys/${keySegment(key)}`);
}

export function deleteKey(key: RedisText): Promise<ActionResult<void>> {
	return call<void>("DELETE", `/api/redis/keys/${keySegment(key)}`);
}

export function listUsers(): Promise<ActionResult<RedisUserList>> {
	return call<RedisUserList>("GET", "/api/redis/users");
}

export function createUser(request: CreateRedisUser): Promise<ActionResult<RedisUser>> {
	return call<RedisUser>("POST", "/api/redis/users", request);
}

export function updateUser(name: string, request: UpdateRedisUser): Promise<ActionResult<RedisUser>> {
	return call<RedisUser>("PUT", `/api/redis/users/${seg(name)}`, request);
}

export function deleteUser(name: string): Promise<ActionResult<void>> {
	return call<void>("DELETE", `/api/redis/users/${seg(name)}`);
}
