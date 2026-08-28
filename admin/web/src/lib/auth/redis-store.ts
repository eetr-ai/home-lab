import "server-only";

/**
 * The Redis side of the coordinated refresh: a client, and the four operations
 * shared-refresh.ts asks for.
 *
 * Kept apart from the rules it serves so those stay testable against a fake. This
 * module is the only one that knows Redis exists, and it holds no logic worth a
 * test of its own beyond the two scripts below.
 */
import Redis from "ioredis";
import type { LockStore } from "./shared-refresh";

const REDIS_URL = process.env.REDIS_URL ?? "";
const REDIS_PASSWORD = process.env.REDIS_PASSWORD ?? "";

/** True when this deployment is configured to coordinate at all. */
export const redisConfigured = REDIS_URL !== "";

let client: Redis | null = null;

/**
 * One client per process, created lazily.
 *
 * `lazyConnect` so importing this module does not open a socket — Next.js
 * evaluates modules during the build, where there is no Redis and no reason to
 * reach for one.
 *
 * `maxRetriesPerRequest: 1` because the caller is a token refresh with a deadline
 * and a fail-closed answer. ioredis's default of 20 retries would sit on a request
 * long past the point where the operator is owed an answer, and the answer after
 * all that waiting is the same one.
 */
function connection(): Redis {
	if (client) return client;
	client = new Redis(REDIS_URL, {
		password: REDIS_PASSWORD || undefined,
		lazyConnect: true,
		maxRetriesPerRequest: 1,
		enableOfflineQueue: false,
		connectTimeout: 2_000,
	});
	// Without a listener a connection error is an unhandled 'error' event, which
	// takes the process down. The operations themselves reject, which is what the
	// caller acts on, so this only has to stop the event being fatal.
	client.on("error", () => {});
	return client;
}

/**
 * Release only what we still hold.
 *
 * A plain DEL is wrong: if this holder's lock already expired — a winner slower
 * than LOCK_TTL_MS — the lock now belongs to somebody else, and deleting it would
 * let a third caller in while the second is mid-exchange. Compare-and-delete has
 * to be atomic, so it is a script rather than a GET followed by a DEL.
 */
const RELEASE = `
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("del", KEYS[1])
end
return 0
`;

export function redisStore(): LockStore {
	return {
		async acquire(key, holder, ttlMs) {
			const set = await connection().set(key, holder, "PX", ttlMs, "NX");
			return set === "OK";
		},
		async release(key, holder) {
			await connection().eval(RELEASE, 1, key, holder);
		},
		async read(key) {
			return await connection().get(key);
		},
		async write(key, value, ttlMs) {
			await connection().set(key, value, "PX", ttlMs);
		},
	};
}
