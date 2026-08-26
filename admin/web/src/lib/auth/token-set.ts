/**
 * The access/refresh token pair the panel holds on behalf of the signed-in
 * operator, and the two rules that govern it. Pure and dependency-free: the
 * network call that uses them lives in refresh.ts.
 *
 * The panel keeps these because it is a client of its own API, which is an OIDC
 * resource server — every call carries the operator's bearer token, and the
 * assistant agent will later carry the same one. That is the whole reason this
 * module exists; a session cookie alone would not reach the API.
 */

/** How long before expiry a token is treated as already expired, in seconds. */
export const REFRESH_SKEW_SECONDS = 60;

/**
 * Assumed access-token lifetime when the token endpoint does not state one.
 * Deliberately short: guessing high means sending dead tokens, and the only cost
 * of guessing low is an extra refresh.
 */
export const DEFAULT_LIFETIME_SECONDS = 300;

/** What the panel stores on the session JWT for the API client to use. */
export interface TokenSet {
	accessToken: string;
	/**
	 * Optional because eetr-auth may omit it from a refresh response, in which
	 * case the one we already hold stays valid.
	 */
	refreshToken?: string;
	/** Epoch seconds. */
	expiresAt: number;
}

/** The token endpoint's success response, as far as this app reads it. */
export interface TokenResponse {
	access_token: string;
	refresh_token?: string;
	expires_in?: number;
}

/**
 * Whether the access token should be renewed before the next request.
 *
 * An unknown expiry counts as expired: a token we cannot date is one we cannot
 * vouch for, and refreshing costs a round trip while guessing wrong costs a
 * failed request.
 */
export function needsRefresh(expiresAt: number | undefined, nowSeconds: number): boolean {
	if (expiresAt === undefined) return true;
	return expiresAt - REFRESH_SKEW_SECONDS <= nowSeconds;
}

/**
 * Fold a token-endpoint response into the stored pair.
 *
 * The rotated refresh token is adopted whenever one comes back, and that is the
 * point of this function: eetr-auth rotates refresh tokens and implements OAuth
 * 2.1 reuse detection, so presenting a superseded one cascade-revokes the whole
 * family and signs the operator out everywhere. Keeping the old value after a
 * successful refresh would guarantee exactly that on the following renewal.
 */
export function nextTokenSet(
	previous: TokenSet,
	response: TokenResponse,
	nowSeconds: number,
): TokenSet {
	const lifetime =
		typeof response.expires_in === "number" && Number.isFinite(response.expires_in)
			? response.expires_in
			: DEFAULT_LIFETIME_SECONDS;

	return {
		accessToken: response.access_token,
		refreshToken: response.refresh_token ?? previous.refreshToken,
		expiresAt: nowSeconds + lifetime,
	};
}
