/**
 * Making one token refresh happen once across every replica.
 *
 * WHY THIS EXISTS. eetr-auth rotates refresh tokens with OAuth 2.1 reuse
 * detection: presenting a superseded one cascade-revokes the whole family and
 * signs the operator out everywhere. Two concurrent requests from one browser
 * carry the *same* cookie — neither response has returned, so neither can have
 * seen the other's rotation — so both find the same expired token and both try to
 * spend it. `refresh.ts` collapses that into one exchange with an in-process Map,
 * which is exactly as far as one process reaches. This is the same idea across
 * processes.
 *
 * THE LOSER CANNOT SIMPLY WAIT AND RETRY. By the time the winner is done, the
 * loser's own refresh token is the superseded one — spending it is the thing being
 * prevented. So the winner has to publish its OUTCOME and the loser has to read
 * it. That is the whole reason this holds a value at all rather than being a plain
 * mutex, and it is why the value is sealed: it carries a live token pair.
 *
 * FAILURES ARE CACHED TOO, and that is not an optimisation. If the winner's
 * exchange was rejected, a loser that fell back to trying for itself would present
 * the same dead token and cause the cascade. A cached failure is what makes the
 * loser fail the same way instead.
 *
 * FAIL CLOSED. Any error from the store answers `ok: false` rather than falling
 * back to an uncoordinated exchange. Both outcomes end with the operator signing
 * in again, so the choice is between a deterministic re-login and a race that can
 * revoke a token family across every device they are signed in on.
 */
import type { RefreshOutcome } from "./refresh";

/**
 * The slice of a key/value store this needs, and nothing more.
 *
 * Narrow on purpose: it keeps the rules below testable against a fake, which is
 * the only way they get tested at all — a suite that needs a live Redis is a
 * suite that does not run.
 */
export interface LockStore {
	/** SET key holder NX PX ttlMs — true when this caller took the lock. */
	acquire(key: string, holder: string, ttlMs: number): Promise<boolean>;
	/** Release only if still held by `holder`, so a lock that expired under a slow
	 *  winner is not deleted out from under whoever holds it now. */
	release(key: string, holder: string): Promise<void>;
	read(key: string): Promise<string | null>;
	write(key: string, value: string, ttlMs: number): Promise<void>;
}

export interface SharedRefreshDeps {
	store: LockStore;
	/** Opaque, stable per refresh token. The token itself is never a key: keys show
	 *  up in MONITOR, in SLOWLOG and in anything that samples them. */
	digest(refreshToken: string): Promise<string>;
	/** The outcome is a live token pair, so it is encrypted before it is stored.
	 *  `open` returns null on anything it cannot authenticate. */
	seal(outcome: RefreshOutcome): Promise<string>;
	open(sealed: string): Promise<RefreshOutcome | null>;
	/** Identifies this caller's claim on the lock. */
	holder: string;
	now(): number;
	sleep(ms: number): Promise<void>;
}

/**
 * How long the winner may hold the lock. Comfortably longer than the exchange's
 * own 10s deadline, so a slow-but-live winner is not overtaken by a waiter that
 * then spends the same token.
 */
export const LOCK_TTL_MS = 15_000;

/**
 * How long an outcome stays readable. Long enough that a waiter delayed behind
 * the winner still finds it, short enough that a live token pair is not sitting in
 * a shared store any longer than the race it exists to settle.
 */
export const OUTCOME_TTL_MS = 30_000;

/** How long a loser waits for the winner's answer before giving up. */
export const WAIT_BUDGET_MS = 12_000;

const POLL_MS = 50;

/**
 * Run `exchange` once for this refresh token across every replica, and give every
 * caller the same answer.
 *
 * Never throws: the caller is Auth.js's `jwt` callback by way of refresh.ts, where
 * a thrown error destroys the session with no explanation rather than reporting
 * one.
 */
export async function sharedRefresh(
	refreshToken: string,
	exchange: () => Promise<RefreshOutcome>,
	deps: SharedRefreshDeps,
): Promise<RefreshOutcome> {
	try {
		return await coordinate(refreshToken, exchange, deps);
	} catch (err) {
		return {
			ok: false,
			error: `token refresh could not be coordinated: ${(err as Error).message}`,
		};
	}
}

async function coordinate(
	refreshToken: string,
	exchange: () => Promise<RefreshOutcome>,
	deps: SharedRefreshDeps,
): Promise<RefreshOutcome> {
	const key = await deps.digest(refreshToken);
	const outcomeKey = `refresh:${key}`;
	const lockKey = `refresh:${key}:lock`;

	// Somebody may already have done this. Checked before reaching for the lock so
	// the common case after a race is one read rather than a lock round trip.
	const cached = await readOutcome(outcomeKey, deps);
	if (cached) return cached;

	const deadline = deps.now() + WAIT_BUDGET_MS;

	for (;;) {
		if (await deps.store.acquire(lockKey, deps.holder, LOCK_TTL_MS)) {
			try {
				// Re-read under the lock. Between the read above and this acquire, a
				// winner may have finished and released — without this the exchange
				// would run a second time, on a token the first run just superseded.
				const settled = await readOutcome(outcomeKey, deps);
				if (settled) return settled;

				const outcome = await exchange();
				// Written before the lock is released, so a waiter that sees the lock
				// gone always finds the answer rather than an empty slot it would read
				// as "the winner died".
				await deps.store.write(outcomeKey, await deps.seal(outcome), OUTCOME_TTL_MS);
				return outcome;
			} finally {
				// Releasing is best-effort, and this catch is load-bearing. The
				// exchange has already happened by now — the token is rotated, the old
				// one is dead — so letting a failed release throw would replace a
				// successful outcome with a failure and sign the operator out over a
				// refresh that worked. The lock expires on its own; the answer does
				// not come round again.
				await deps.store.release(lockKey, deps.holder).catch(() => {});
			}
		}

		// Somebody else is exchanging. Wait for what they publish.
		await deps.sleep(POLL_MS);

		const answer = await readOutcome(outcomeKey, deps);
		if (answer) return answer;

		if (deps.now() >= deadline) {
			return {
				ok: false,
				error: "token refresh timed out waiting for another replica",
			};
		}
		// Otherwise loop: the lock may have expired because its holder died, and
		// this caller is entitled to try for it.
	}
}

/**
 * Read and decrypt an outcome, treating anything unreadable as absent.
 *
 * A value that will not open is a key sealed under a different AUTH_SECRET — a
 * rotation, or two deployments sharing a Redis. Treating it as a miss lets the
 * caller take the lock and produce a fresh one, which is what recovers; treating
 * it as an error would make every refresh fail until the key expired.
 */
async function readOutcome(
	key: string,
	deps: SharedRefreshDeps,
): Promise<RefreshOutcome | null> {
	const raw = await deps.store.read(key);
	if (!raw) return null;
	return await deps.open(raw);
}
