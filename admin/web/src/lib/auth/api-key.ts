import { OAuthError, exchangeApiKey } from "@eetr/eetr-auth-client";
import { AUDIENCE, ISSUER } from "./oidc-config";

/**
 * Turning a pipeline's API key into an access token the admin API accepts.
 *
 * This is the other half of `PATCH /api/v1/charts/{id}`: a pipeline holds one
 * opaque credential, and the panel is the only thing that knows how to spend it.
 * The alternative — giving CI a client secret and letting it mint tokens against
 * an admin API that would then have to be publicly routable — is the arrangement
 * this replaces.
 *
 * No `server-only` here, for the same reason `refresh.ts` has none though it
 * holds the client secret: that module throws when it is merely imported outside
 * a server bundle, which takes the unit tests with it. What keeps this off the
 * client is that its only caller is a route handler, and that nothing it exports
 * is useful in a browser — a key arrives on a request header this side of the
 * wire or not at all.
 */

/** The prefix eetr-auth gives every API key: `eak_<keyId>_<secret>`. */
const KEY_PREFIX = "eak_";

/**
 * How the exchange is performed. Injected so the tests can run without a network,
 * the way `refresh.ts` takes its `fetch`.
 */
export interface ExchangeDeps {
	exchange: typeof exchangeApiKey;
	issuer: string;
	/**
	 * The value the admin API requires in `aud`, or "" to ask for nothing.
	 *
	 * eetr-auth puts the client id in `aud` when no resource indicator is
	 * requested, so a key issued on the panel's own client already lands on the
	 * right audience whatever this says.
	 *
	 * It is only *asked for* when it is an absolute URI — see `resourceFor`, which
	 * is where the reasoning is, and which exists because asking for a client id
	 * is a `400` rather than a no-op.
	 */
	audience: string;
}

/**
 * What a caller gets: a token to spend, or a refusal and the status it deserves.
 *
 * The status is carried because the two failures here are genuinely different to
 * whoever is debugging a red pipeline. A key the provider rejected is a 401 and a
 * provider that could not be reached is a 502, and answering 401 for both would
 * send someone to rotate a credential that was never the problem.
 */
export type Exchanged = { token: string } | { error: string; status: 401 | 502 };

/**
 * The one thing said about every failed key.
 *
 * eetr-auth answers `401 invalid_client` for malformed, unknown, revoked,
 * expired, and wrong-secret alike, deliberately, so the endpoint cannot be used
 * to enumerate key ids. Repeating that discretion here is the point: a panel that
 * distinguished "revoked" from "never existed" would undo it.
 */
const REFUSED = "the API key was not accepted";

/**
 * The API key on a request, or nothing.
 *
 * Shape only. Whether the key is real is eetr-auth's answer to give, and asking
 * it is a round trip — but a header that is not a bearer token at all, or a
 * bearer that is not an API key, is a question not worth asking.
 */
export function apiKeyFrom(header: string | null): string | undefined {
	if (!header) return undefined;
	const [scheme, ...rest] = header.trim().split(/\s+/);
	if (scheme?.toLowerCase() !== "bearer" || rest.length !== 1) return undefined;
	const key = rest[0];
	return key?.startsWith(KEY_PREFIX) && key.length > KEY_PREFIX.length ? key : undefined;
}

/**
 * Where keys are exchanged, derived from the issuer.
 *
 * The trailing slash is trimmed here and nowhere else. `ISSUER` is kept
 * byte-for-byte because it is an *identifier* the API compares tokens against,
 * and normalizing it there would reject every token from a provider that spells
 * its own with a slash. This is URL construction rather than comparison, so the
 * two are not the same rule — do not "fix" one to match the other.
 */
export function apiKeyEndpoint(issuer: string): string {
	return `${issuer.replace(/\/+$/, "")}/api/token/api-key`;
}

/**
 * Exchange an API key for an access token.
 *
 * Never throws, and never caches. A cache would be a way to keep honouring a key
 * that was revoked a minute ago, and the traffic here is one request per deploy —
 * so the round trip buys revocation that takes effect immediately and costs
 * nothing anybody will measure.
 */
export async function exchangeForToken(
	apiKey: string,
	deps: ExchangeDeps = defaultDeps(),
): Promise<Exchanged> {
	if (!deps.issuer) {
		return {
			error: "the identity provider is not configured (OIDC_ISSUER is unset)",
			status: 502,
		};
	}

	try {
		const response = await deps.exchange(
			// No `scope`: the admin API reads none, and asking for one a key does not
			// hold is an `invalid_scope` in exchange for nothing.
			{ apiKey, ...resourceFor(deps.audience) },
			{ apiKeyEndpoint: apiKeyEndpoint(deps.issuer) },
		);
		return response.access_token
			? { token: response.access_token }
			: { error: REFUSED, status: 401 };
	} catch (err) {
		if (err instanceof OAuthError) {
			// `invalid_client` is every reason a key can be bad — unknown, revoked,
			// expired, wrong secret — collapsed into one answer on purpose, so this
			// branch says no more than eetr-auth itself does.
			if (err.status === 401 || err.code === "invalid_client") {
				return { error: REFUSED, status: 401 };
			}
			// Anything else is the panel having asked for something wrong, and
			// reporting it as a bad key sends somebody to rotate a credential that
			// was fine. The provider's own words, because they name the field.
			return {
				error: `the identity provider refused the exchange (${err.code})${
					err.description ? `: ${err.description}` : ""
				}`,
				status: 502,
			};
		}
		return {
			error: `the identity provider is unreachable: ${(err as Error).message}`,
			status: 502,
		};
	}
}

/**
 * Whether the configured audience can be asked for as a resource indicator, and
 * the whole of why this is not just `audience ? {resource: audience} : {}`.
 *
 * RFC 8707 requires an **absolute URI**, and eetr-auth enforces it: a bare client
 * id comes back `400 invalid_target`, "resource must be an absolute URI". That is
 * the normal configuration here — `admin.api.oidc.audience` is documented as the
 * panel's client id — and it needs no resource at all, because a token minted
 * without one already carries the client id in `aud`. So the common case must ask
 * for nothing, and only an audience that is genuinely a URI is passed through.
 *
 * A fragment disqualifies it too. RFC 8707 says the resource "MUST NOT include a
 * fragment component", and `new URL` is perfectly happy with one — so without
 * this check a `https://api.example/#x` audience would sail through here and come
 * back as the same `invalid_target` this function exists to prevent.
 */
export function resourceFor(audience: string): { resource: string } | undefined {
	if (!audience) return undefined;
	// The raw string rather than `url.hash`, which is empty for a bare trailing
	// "#" — a URI that still carries a fragment component as far as the RFC is
	// concerned, and as far as a strict provider is likely to be.
	if (audience.includes("#")) return undefined;
	try {
		// A scheme is what makes it absolute. `new URL` needs no base for one, and
		// throws for anything else — including a client id with an underscore in it.
		return new URL(audience).protocol ? { resource: audience } : undefined;
	} catch {
		return undefined;
	}
}

function defaultDeps(): ExchangeDeps {
	return { exchange: exchangeApiKey, issuer: ISSUER, audience: AUDIENCE };
}
