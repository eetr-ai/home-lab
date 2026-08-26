/**
 * Renewing the operator's access token at eetr-auth.
 *
 * The panel is a client of its own API, and that API is an OIDC resource server:
 * every call carries the operator's bearer token. Access tokens are short-lived,
 * so this is what keeps a session usable for longer than an hour.
 *
 * Two properties matter more than anything else here, and both come from
 * eetr-auth rotating refresh tokens with OAuth 2.1 reuse detection — presenting a
 * superseded refresh token cascade-revokes the entire family and signs the
 * operator out everywhere:
 *
 *  1. **A failed refresh is never retried.** It reports the failure and the
 *     session is forced back through sign-in.
 *  2. **Concurrent refreshes of the same token collapse into one exchange.** Two
 *     parallel server actions finding the same expired token would otherwise
 *     present it twice, which is precisely the replay reuse detection punishes.
 *
 * The single-flight below is per process. That is enough while the web runs at
 * one replica (see charts/admin/values.yaml), and it is the seam to replace with
 * a shared lock — Redis, say — if it ever runs at more.
 */
import { nextTokenSet, type TokenResponse, type TokenSet } from "./token-set";

/** Everything the exchange needs, injected so the rules can be tested directly. */
export interface RefreshDeps {
	tokenEndpoint: string;
	clientId: string;
	clientSecret: string;
	fetch: typeof globalThis.fetch;
	/** Epoch seconds. */
	now: () => number;
}

export type RefreshOutcome = { ok: true; tokens: TokenSet } | { ok: false; error: string };

// Keyed by refresh token, so two different sessions never wait on each other.
const inFlight = new Map<string, Promise<RefreshOutcome>>();

/**
 * Exchange the stored refresh token for a new pair.
 *
 * Never throws: the caller is Auth.js's `jwt` callback, where a thrown error
 * destroys the session with no explanation rather than reporting one.
 */
export function refreshTokenSet(previous: TokenSet, deps: RefreshDeps): Promise<RefreshOutcome> {
	const { refreshToken } = previous;
	if (!refreshToken) {
		return Promise.resolve({ ok: false, error: "the session holds no refresh token" });
	}

	const running = inFlight.get(refreshToken);
	if (running) return running;

	const exchange = exchangeOnce(previous, refreshToken, deps).finally(() => {
		// Cleared on settle, not on success: leaving the entry would pin every later
		// refresh of this session to one stale answer.
		inFlight.delete(refreshToken);
	});
	inFlight.set(refreshToken, exchange);
	return exchange;
}

async function exchangeOnce(
	previous: TokenSet,
	refreshToken: string,
	deps: RefreshDeps,
): Promise<RefreshOutcome> {
	const body = new URLSearchParams({
		grant_type: "refresh_token",
		refresh_token: refreshToken,
	});

	let res: Response;
	try {
		res = await deps.fetch(deps.tokenEndpoint, {
			method: "POST",
			headers: {
				"Content-Type": "application/x-www-form-urlencoded",
				// client_secret_basic. eetr-auth registers this panel as a confidential
				// client, and the header keeps the secret out of anything that logs a
				// request body.
				Authorization: `Basic ${basicCredentials(deps.clientId, deps.clientSecret)}`,
			},
			body: body.toString(),
		});
	} catch (err) {
		return { ok: false, error: `token refresh failed: ${(err as Error).message}` };
	}

	if (!res.ok) {
		return { ok: false, error: `token refresh rejected (${res.status})` };
	}

	let parsed: TokenResponse;
	try {
		parsed = (await res.json()) as TokenResponse;
	} catch {
		return { ok: false, error: "token refresh returned a body that is not JSON" };
	}
	if (!parsed.access_token) {
		return { ok: false, error: "token refresh returned no access token" };
	}

	return { ok: true, tokens: nextTokenSet(previous, parsed, deps.now()) };
}

/**
 * Base64 of `client_id:client_secret`, per RFC 6749 §2.3.1.
 *
 * `btoa` rather than `Buffer`, because this module is reachable from the
 * edge-safe auth config where `Buffer` may not exist. Both are ASCII here — the
 * client id and secret come from eetr-auth, which issues neither with non-ASCII
 * characters.
 */
function basicCredentials(clientId: string, clientSecret: string): string {
	return btoa(`${clientId}:${clientSecret}`);
}
