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
 * The single-flight below is per process, and it is now the FIRST of two tiers
 * rather than the only one. It still earns its place: two requests landing on the
 * same pod are collapsed here without a network round trip. What it cannot do is
 * see the other replicas, so `coordinate` below hands the exchange to a shared
 * lock when one is configured — see shared-refresh.ts.
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
	/** Overridden in tests; defaults to {@link EXCHANGE_TIMEOUT_MS}. */
	timeoutMs?: number;
	/**
	 * Runs the exchange once across every replica and gives each caller the same
	 * answer. Absent means single-process behaviour: the in-process map below is
	 * the whole of the deduplication, which is correct only at one replica.
	 */
	coordinate?: (
		refreshToken: string,
		exchange: () => Promise<RefreshOutcome>,
	) => Promise<RefreshOutcome>;
}

/**
 * How long one exchange may take.
 *
 * A deadline rather than patience: the single-flight entry below is keyed on the
 * refresh token, so an exchange that never settles is not one slow request — it
 * is every later refresh of that session waiting on the same stuck promise, for
 * as long as the process lives.
 */
export const EXCHANGE_TIMEOUT_MS = 10_000;

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

	// The coordinator wraps the exchange rather than replacing it, so the rules
	// above — the deadline, the credential, what counts as a failure — are the same
	// whether or not one is configured.
	const once = () => exchangeOnce(previous, refreshToken, deps);
	const attempt = deps.coordinate ? deps.coordinate(refreshToken, once) : once();

	const exchange = attempt.finally(() => {
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

	const deadline = new AbortController();
	const timer = setTimeout(() => deadline.abort(), deps.timeoutMs ?? EXCHANGE_TIMEOUT_MS);

	try {
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
				signal: deadline.signal,
			});
		} catch (err) {
			return { ok: false, error: `token refresh failed: ${(err as Error).message}` };
		}

		if (!res.ok) {
			return { ok: false, error: `token refresh rejected (${res.status})` };
		}

		let parsed: TokenResponse;
		try {
			// Inside the deadline on purpose: a response whose headers arrive and whose
			// body never does would otherwise hang here instead.
			parsed = (await res.json()) as TokenResponse;
		} catch {
			return { ok: false, error: "token refresh returned a body that is not JSON" };
		}
		if (!parsed.access_token) {
			return { ok: false, error: "token refresh returned no access token" };
		}

		return { ok: true, tokens: nextTokenSet(previous, parsed, deps.now()) };
	} finally {
		clearTimeout(timer);
	}
}

/**
 * Base64 of `client_id:client_secret`, per RFC 6749 §2.3.1 — which requires both
 * halves to be form-urlencoded *before* they are joined and encoded.
 *
 * Matching that matters for a reason beyond conformance: Auth.js performs the
 * other half of this exchange, and its oauth4webapi encodes the credentials
 * exactly this way when it redeems the authorization code. Skipping the encoding
 * here would mean sign-in and refresh presenting different credentials for the
 * same secret — working for one and failing for the other, which is a far worse
 * thing to debug than either failing outright.
 *
 * `btoa` rather than `Buffer`, because this module is reachable from the
 * edge-safe auth config where `Buffer` may not exist. Percent-encoding first also
 * guarantees the input is ASCII, which `btoa` requires.
 */
function basicCredentials(clientId: string, clientSecret: string): string {
	return btoa(`${formUrlEncode(clientId)}:${formUrlEncode(clientSecret)}`);
}

/**
 * `application/x-www-form-urlencoded` encoding of one value, per RFC 6749
 * Appendix B: `encodeURIComponent`, plus the characters it leaves alone that the
 * form encoding does not, and a space as `+`.
 */
function formUrlEncode(value: string): string {
	return encodeURIComponent(value)
		.replace(/[-_.!~*'()]/g, (char) => `%${char.charCodeAt(0).toString(16).toUpperCase()}`)
		.replace(/%20/g, "+");
}
