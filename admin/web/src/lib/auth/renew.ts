/**
 * The one place that turns "this token is stale" into a real exchange with
 * eetr-auth: it resolves the token endpoint by discovery and supplies the
 * client credentials, then delegates to the rules in refresh.ts.
 *
 * Kept apart from those rules so they stay testable without an environment, and
 * so this stays the single seam to replace if the token pair ever moves out of
 * the session cookie into a shared store.
 */
import { CLIENT_ID, CLIENT_SECRET } from "./oidc-config";
import { discover } from "./discovery";
import { refreshTokenSet, type RefreshOutcome } from "./refresh";
import type { TokenSet } from "./token-set";

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
	});
}
