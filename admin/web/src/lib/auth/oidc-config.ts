/**
 * The panel's OIDC configuration, read from the environment in one place.
 *
 * Two things depend on it and must not drift apart: Auth.js, which signs the
 * operator in, and the token refresh, which renews the access token the API
 * client sends. Both read from here.
 *
 * Dependency-free on purpose — the edge-safe auth config imports it, and that
 * bundle cannot afford anything heavier.
 */

/**
 * The Auth.js provider id, and with it the callback path
 * `{AUTH_URL}/api/auth/callback/eetr` registered with eetr-auth.
 *
 * A constant rather than a knob: it is not user-visible, and making it
 * configurable would only add a way to silently invalidate the registered
 * redirect URI, which eetr-auth matches exactly.
 */
export const PROVIDER_ID = "eetr";

/** How the provider is named on the sign-in button. */
export const PROVIDER_NAME = process.env.OIDC_PROVIDER_NAME || "Eetr Auth";

/**
 * The issuer URL, kept byte-for-byte as configured — trailing slash and all. It
 * is an identifier rather than a path, and the API checks tokens against the
 * same string, so normalizing it on one side only would reject every token.
 */
export const ISSUER = process.env.OIDC_ISSUER ?? "";

/**
 * The audience the admin API requires in a token, when it requires one.
 *
 * Only the pipeline endpoint reads it: an operator's token comes from a sign-in
 * on this very client and already carries the right `aud`, while a token minted
 * from an API key may not. Empty is a valid and common configuration — the API
 * skips the audience check when its own is unset, and eetr-auth puts the client
 * id there by default. Declared as a plain string rather than `undefined` so the
 * exchange can ask "is there one" without a second spelling of empty.
 */
export const AUDIENCE = env("OIDC_AUDIENCE") ?? "";

export const CLIENT_ID = env("OIDC_CLIENT_ID");
export const CLIENT_SECRET = env("OIDC_CLIENT_SECRET");

/**
 * Scopes requested at sign-in. `offline_access` is not among them: eetr-auth
 * issues a refresh token for a confidential client without it, and asking for a
 * scope the provider does not advertise is a way to fail authorization for no
 * gain.
 */
export const SCOPES = env("OIDC_SCOPES") ?? "openid profile email";

/** Read an env var, returning undefined rather than "" when unset or blank. */
function env(name: string): string | undefined {
	const value = process.env[name];
	return value && value !== "" ? value : undefined;
}
