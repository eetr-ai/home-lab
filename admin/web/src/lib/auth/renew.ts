/**
 * The one place that turns "this token is stale" into a real exchange with
 * eetr-auth: it resolves the token endpoint by discovery, supplies the client
 * credentials, and — when a shared store is configured — the coordinator that
 * makes the exchange happen once across every replica.
 *
 * Kept apart from the rules in refresh.ts so those stay testable without an
 * environment. This is the seam the shared store plugs into, which is what it was
 * held apart for.
 */
import "server-only";

import { CLIENT_ID, CLIENT_SECRET } from "./oidc-config";
import { discover } from "./discovery";
import { refreshTokenSet, type RefreshOutcome } from "./refresh";
import { digestToken, openOutcome, sealOutcome } from "./outcome-seal";
import { redisConfigured, redisStore } from "./redis-store";
import { sharedRefresh } from "./shared-refresh";
import type { TokenSet } from "./token-set";

/**
 * Identifies this process's claim on a lock. Random per process rather than a
 * pod name: it only has to be unique among concurrent holders, and reading a
 * hostname is one more thing to be unset in a test.
 */
const HOLDER = crypto.randomUUID();

/**
 * The coordinator, or undefined when this deployment runs a single replica and
 * needs none.
 *
 * AUTH_SECRET is required rather than optional: it is what seals the token pair
 * before it is shared, and coordinating without it would put live credentials in
 * Redis in the clear. A deployment with REDIS_URL and no AUTH_SECRET cannot sign
 * anybody in anyway — Auth.js requires it — so this cannot be the reason a working
 * panel stops working.
 */
function coordinator() {
	const secret = process.env.AUTH_SECRET ?? "";
	if (!redisConfigured || !secret) return undefined;

	const store = redisStore();
	return (refreshToken: string, exchange: () => Promise<RefreshOutcome>) =>
		sharedRefresh(refreshToken, exchange, {
			store,
			digest: digestToken,
			seal: (outcome) => sealOutcome(outcome, secret),
			open: (sealed) => openOutcome(sealed, secret),
			holder: HOLDER,
			now: () => Date.now(),
			sleep: (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
		});
}

/** Renew `previous` against the configured provider. Never throws. */
export async function renew(previous: TokenSet): Promise<RefreshOutcome> {
	if (!CLIENT_ID || !CLIENT_SECRET) {
		return { ok: false, error: "OIDC_CLIENT_ID or OIDC_CLIENT_SECRET is not set" };
	}

	let tokenEndpoint: string;
	try {
		({ token_endpoint: tokenEndpoint } = await discover());
	} catch (err) {
		return { ok: false, error: `token refresh failed: ${(err as Error).message}` };
	}

	return refreshTokenSet(previous, {
		tokenEndpoint,
		clientId: CLIENT_ID,
		clientSecret: CLIENT_SECRET,
		fetch: globalThis.fetch,
		now: () => Math.floor(Date.now() / 1000),
		coordinate: coordinator(),
	});
}
