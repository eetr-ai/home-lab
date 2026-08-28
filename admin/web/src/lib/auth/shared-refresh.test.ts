import { describe, expect, it, vi } from "vitest";
import {
	sharedRefresh,
	WAIT_BUDGET_MS,
	type LockStore,
	type SharedRefreshDeps,
} from "./shared-refresh";
import type { RefreshOutcome } from "./refresh";
import { digestToken } from "./outcome-seal";

/**
 * A fake store rather than a Redis.
 *
 * It is deliberately not a correct Redis — it is a correct *lock*: acquire is
 * compare-and-set on absence, release only removes what the holder still owns,
 * and expiry is driven by the same clock the code under test reads. That is the
 * whole of what these rules depend on, and it means the suite runs anywhere.
 */
function fakeStore(clock: { now: number }) {
	const entries = new Map<string, { value: string; expiresAt: number }>();

	const live = (key: string) => {
		const entry = entries.get(key);
		if (!entry) return null;
		if (entry.expiresAt <= clock.now) {
			entries.delete(key);
			return null;
		}
		return entry;
	};

	const store: LockStore = {
		async acquire(key, holder, ttlMs) {
			if (live(key)) return false;
			entries.set(key, { value: holder, expiresAt: clock.now + ttlMs });
			return true;
		},
		async release(key, holder) {
			if (live(key)?.value === holder) entries.delete(key);
		},
		async read(key) {
			return live(key)?.value ?? null;
		},
		async write(key, value, ttlMs) {
			entries.set(key, { value, expiresAt: clock.now + ttlMs });
		},
	};
	return { store, entries };
}

/** Seals as readable JSON: what is under test here is the coordination, not the
 *  encryption, which outcome-seal.test.ts covers on its own. */
function deps(
	store: LockStore,
	clock: { now: number },
	overrides: Partial<SharedRefreshDeps> = {},
): SharedRefreshDeps {
	return {
		store,
		digest: async (token) => `digest(${token})`,
		seal: async (outcome) => JSON.stringify(outcome),
		open: async (sealed) => JSON.parse(sealed) as RefreshOutcome,
		holder: "holder-1",
		now: () => clock.now,
		// Advances the same clock the store expires against, so a test never waits
		// in real time and expiry is deterministic.
		sleep: async (ms) => {
			clock.now += ms;
		},
		...overrides,
	};
}

const OK: RefreshOutcome = {
	ok: true,
	tokens: { accessToken: "new-access", refreshToken: "new-refresh", expiresAt: 42 },
};

describe("sharedRefresh", () => {
	it("exchanges once and returns the outcome", async () => {
		const clock = { now: 1_000 };
		const { store } = fakeStore(clock);
		const exchange = vi.fn(async () => OK);

		const got = await sharedRefresh("rt-1", exchange, deps(store, clock));

		expect(got).toEqual(OK);
		expect(exchange).toHaveBeenCalledTimes(1);
	});

	// The whole point. Two replicas, one token: the second must not spend it.
	it("gives a concurrent caller the winner's outcome without exchanging again", async () => {
		const clock = { now: 1_000 };
		const { store } = fakeStore(clock);

		let release!: () => void;
		const held = new Promise<void>((resolve) => {
			release = resolve;
		});
		const winnerExchange = vi.fn(async () => {
			await held;
			return OK;
		});
		const loserExchange = vi.fn(async () => OK);

		const winner = sharedRefresh("rt-1", winnerExchange, deps(store, clock));
		const loser = sharedRefresh("rt-1", loserExchange, deps(store, clock, { holder: "holder-2" }));

		release();
		expect(await winner).toEqual(OK);
		expect(await loser).toEqual(OK);

		expect(winnerExchange).toHaveBeenCalledTimes(1);
		// The assertion that matters: the loser never presented the superseded token.
		expect(loserExchange).not.toHaveBeenCalled();
	});

	// A cached failure is not an optimisation. Without it the loser falls back to
	// exchanging for itself, presents the token the winner just had rejected, and
	// causes exactly the cascade this exists to prevent.
	it("shares a failure too, so the loser does not retry a dead token", async () => {
		const clock = { now: 1_000 };
		const { store } = fakeStore(clock);
		const failure: RefreshOutcome = { ok: false, error: "token refresh rejected (400)" };

		const first = await sharedRefresh("rt-1", async () => failure, deps(store, clock));
		const loserExchange = vi.fn(async () => OK);
		const second = await sharedRefresh("rt-1", loserExchange, deps(store, clock));

		expect(first).toEqual(failure);
		expect(second).toEqual(failure);
		expect(loserExchange).not.toHaveBeenCalled();
	});

	it("reads a settled outcome without taking the lock at all", async () => {
		const clock = { now: 1_000 };
		const { store } = fakeStore(clock);
		await sharedRefresh("rt-1", async () => OK, deps(store, clock));

		const acquire = vi.spyOn(store, "acquire");
		const exchange = vi.fn(async () => OK);
		const got = await sharedRefresh("rt-1", exchange, deps(store, clock));

		expect(got).toEqual(OK);
		expect(exchange).not.toHaveBeenCalled();
		expect(acquire).not.toHaveBeenCalled();
	});

	// Different tokens are different conversations and must not serialize.
	it("does not let one token's refresh block another's", async () => {
		const clock = { now: 1_000 };
		const { store } = fakeStore(clock);
		const exchange = vi.fn(async () => OK);

		await Promise.all([
			sharedRefresh("rt-1", exchange, deps(store, clock)),
			sharedRefresh("rt-2", exchange, deps(store, clock)),
		]);

		expect(exchange).toHaveBeenCalledTimes(2);
	});

	// A winner that died holding the lock. The lock expires, and the waiter is
	// entitled to take it rather than waiting out its whole budget.
	it("takes over when the lock expires with no outcome written", async () => {
		const clock = { now: 1_000 };
		const { store } = fakeStore(clock);
		await store.acquire("refresh:digest(rt-1):lock", "dead-holder", 200);

		const exchange = vi.fn(async () => OK);
		const got = await sharedRefresh("rt-1", exchange, deps(store, clock));

		expect(got).toEqual(OK);
		expect(exchange).toHaveBeenCalledTimes(1);
	});

	// Fail closed. Both outcomes end in a re-login, so the choice is between a
	// deterministic one and a race that can revoke the family on every device.
	it("fails rather than exchanging uncoordinated when the store is unreachable", async () => {
		const clock = { now: 1_000 };
		const exchange = vi.fn(async () => OK);
		const broken: LockStore = {
			acquire: async () => {
				throw new Error("ECONNREFUSED");
			},
			release: async () => {},
			read: async () => null,
			write: async () => {},
		};

		const got = await sharedRefresh("rt-1", exchange, deps(broken, clock));

		expect(got.ok).toBe(false);
		expect(exchange).not.toHaveBeenCalled();
		if (!got.ok) expect(got.error).toContain("ECONNREFUSED");
	});

	// The exchange has already happened when the release runs: the token is
	// rotated and the old one is dead. Letting a failed release throw would replace
	// a successful outcome with a failure and sign the operator out over a refresh
	// that worked — and the answer does not come round again.
	it("keeps a successful outcome when releasing the lock fails", async () => {
		const clock = { now: 1_000 };
		const { store } = fakeStore(clock);
		const releasing: LockStore = {
			...store,
			release: async () => {
				throw new Error("connection reset");
			},
		};

		const got = await sharedRefresh("rt-1", async () => OK, deps(releasing, clock));

		expect(got).toEqual(OK);
	});

	// ...and the outcome is still readable afterwards, so the waiters this exists
	// for are served even though the lock was left behind to expire on its own.
	it("still publishes the outcome when the release fails", async () => {
		const clock = { now: 1_000 };
		const { store } = fakeStore(clock);
		const releasing: LockStore = {
			...store,
			release: async () => {
				throw new Error("connection reset");
			},
		};
		await sharedRefresh("rt-1", async () => OK, deps(releasing, clock));

		const loserExchange = vi.fn(async () => OK);
		const second = await sharedRefresh("rt-1", loserExchange, deps(releasing, clock));

		expect(second).toEqual(OK);
		expect(loserExchange).not.toHaveBeenCalled();
	});

	it("gives up rather than waiting forever on a lock nobody releases", async () => {
		const clock = { now: 1_000 };
		const { store } = fakeStore(clock);
		// Held far longer than the waiter's budget, and never written to.
		await store.acquire("refresh:digest(rt-1):lock", "other", WAIT_BUDGET_MS * 10);

		const exchange = vi.fn(async () => OK);
		const got = await sharedRefresh("rt-1", exchange, deps(store, clock));

		expect(got.ok).toBe(false);
		expect(exchange).not.toHaveBeenCalled();
	});

	// An unreadable value is a rotated AUTH_SECRET, or two deployments sharing one
	// Redis. Recovering by exchanging beats failing every refresh until it expires.
	it("treats an outcome it cannot open as absent", async () => {
		const clock = { now: 1_000 };
		const { store } = fakeStore(clock);
		await store.write("refresh:digest(rt-1)", "not-openable", 30_000);

		const exchange = vi.fn(async () => OK);
		const got = await sharedRefresh(
			"rt-1",
			exchange,
			deps(store, clock, { open: async () => null }),
		);

		expect(got).toEqual(OK);
		expect(exchange).toHaveBeenCalledTimes(1);
	});

	// The refresh token is a credential; a key is the part of a store that leaks
	// most readily, through MONITOR, SLOWLOG and KEYS.
	it("never uses the refresh token itself as a key", async () => {
		const clock = { now: 1_000 };
		const { store, entries } = fakeStore(clock);

		// The REAL digest here, deliberately. Every other case uses a readable fake
		// so failures name the token they are about, but this assertion is about the
		// digest actually keeping the token out of a key — against the fake it would
		// only be testing the fake, and it failed exactly that way when written.
		await sharedRefresh(
			"super-secret-token",
			async () => OK,
			deps(store, clock, { digest: digestToken }),
		);

		for (const key of entries.keys()) {
			expect(key).not.toContain("super-secret-token");
		}
		expect(entries.size).toBeGreaterThan(0);
	});
});
