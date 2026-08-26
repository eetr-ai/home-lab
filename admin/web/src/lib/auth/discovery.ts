/**
 * OpenID Connect discovery, for the one thing Auth.js does not hand back: the
 * token endpoint, which the refresh needs.
 *
 * Auth.js discovers the same document for itself, but keeps the result private,
 * so this asks again and caches the answer for the lifetime of the process.
 */
import { ISSUER } from "./oidc-config";

/** The fields of the discovery document this app reads. */
export interface ProviderMetadata {
	token_endpoint: string;
}

/**
 * The discovery URL for an issuer.
 *
 * Only the appending trims the trailing slash — the issuer itself is kept exactly
 * as configured, because it is an identifier the API compares tokens against and
 * normalizing it on one side would reject every one of them.
 */
export function wellKnownUrl(issuer: string): string {
	if (!issuer) throw new Error("OIDC_ISSUER is not set");
	return `${issuer.replace(/\/+$/, "")}/.well-known/openid-configuration`;
}

// Cached as the in-flight promise rather than the resolved value, so concurrent
// callers during startup share one request instead of racing to make their own.
// Cleared on failure so a provider that was briefly unreachable is retried.
let metadata: Promise<ProviderMetadata> | undefined;

/** The provider's metadata, fetched once per process. */
export function discover(): Promise<ProviderMetadata> {
	metadata ??= fetchMetadata().catch((err: unknown) => {
		metadata = undefined;
		throw err;
	});
	return metadata;
}

async function fetchMetadata(): Promise<ProviderMetadata> {
	const res = await fetch(wellKnownUrl(ISSUER));
	if (!res.ok) {
		throw new Error(`OIDC discovery failed (${res.status})`);
	}
	const body = (await res.json()) as Partial<ProviderMetadata>;
	if (!body.token_endpoint) {
		throw new Error("OIDC discovery returned no token_endpoint");
	}
	return { token_endpoint: body.token_endpoint };
}
