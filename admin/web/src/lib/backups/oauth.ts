/**
 * The browser half of connecting Google Drive: PKCE, the consent URL, and the
 * short-lived cookie that carries the flow across the trip to Google.
 *
 * The code is exchanged by the API, which owns everything Google. What the panel
 * owns is proving that the callback finishing a flow is the browser that started
 * it — `state` against cross-site request forgery, the PKCE verifier against a
 * stolen code — and both are kept in a cookie only this deployment can open.
 */

const GOOGLE_AUTH_URL = "https://accounts.google.com/o/oauth2/v2/auth";

/** The path Google sends the browser back to, registered on the OAuth client. */
export const CALLBACK_PATH = "/api/backups/google/callback";

/**
 * The redirect URI for this panel, from AUTH_URL.
 *
 * Its origin only, whatever AUTH_URL carries after it: Google matches the URI
 * byte for byte against the one registered, so a trailing slash written into a
 * values file must not become a refused sign-in. Null when AUTH_URL is not a URL,
 * which is a misconfiguration to say out loud rather than a redirect to guess.
 */
export function redirectUriFor(authUrl: string | undefined): string | null {
	try {
		return new URL(CALLBACK_PATH, new URL(authUrl ?? "").origin).toString();
	} catch {
		return null;
	}
}

/** The flow cookie's name, and how long a trip to Google may take. */
export const FLOW_COOKIE = "backups-google-flow";
export const FLOW_TTL_SECONDS = 600;

/** What the cookie carries between leaving for Google and coming back. */
export interface Flow {
	state: string;
	verifier: string;
	/** Milliseconds since the epoch. */
	expiresAt: number;
}

const VERIFIER_BYTES = 48;
const STATE_BYTES = 24;
const NONCE_BYTES = 12;

/** A fresh flow: random state, a random 64-character verifier, and its expiry. */
export function newFlow(now: number): Flow {
	return {
		state: base64url(crypto.getRandomValues(new Uint8Array(STATE_BYTES))),
		verifier: base64url(crypto.getRandomValues(new Uint8Array(VERIFIER_BYTES))),
		expiresAt: now + FLOW_TTL_SECONDS * 1000,
	};
}

/** The S256 challenge for a verifier (RFC 7636 §4.2). */
export async function challengeFor(verifier: string): Promise<string> {
	const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier));
	return base64url(new Uint8Array(digest));
}

/**
 * The consent URL.
 *
 * `access_type=offline` is what yields a refresh token, which is the whole point:
 * the backups run at three in the morning with nobody signed in. `prompt=consent`
 * makes Google hand one out even to an account that granted this client before —
 * without it a reconnect gets an access token and nothing to keep.
 */
export function authorizationUrl(options: {
	clientId: string;
	redirectUri: string;
	scopes: string[];
	state: string;
	challenge: string;
}): string {
	const params = new URLSearchParams({
		client_id: options.clientId,
		redirect_uri: options.redirectUri,
		response_type: "code",
		scope: options.scopes.join(" "),
		state: options.state,
		code_challenge: options.challenge,
		code_challenge_method: "S256",
		access_type: "offline",
		prompt: "consent",
		include_granted_scopes: "false",
	});
	return `${GOOGLE_AUTH_URL}?${params.toString()}`;
}

/**
 * Seal a flow for the cookie.
 *
 * AES-256-GCM under a key derived from AUTH_SECRET, the same arrangement
 * outcome-seal.ts uses for refresh outcomes. The derivation is domain-separated
 * with a label, so a value sealed for one purpose never opens as the other.
 */
export async function sealFlow(flow: Flow, secret: string): Promise<string> {
	const key = await flowKey(secret);
	const nonce = crypto.getRandomValues(new Uint8Array(NONCE_BYTES));
	const ciphertext = new Uint8Array(
		await crypto.subtle.encrypt(
			{ name: "AES-GCM", iv: nonce },
			key,
			new TextEncoder().encode(JSON.stringify(flow)),
		),
	);
	const joined = new Uint8Array(nonce.length + ciphertext.length);
	joined.set(nonce);
	joined.set(ciphertext, nonce.length);
	return base64url(joined);
}

/**
 * Open a sealed flow, or null for anything that is not a live one this
 * deployment sealed: tampered, truncated, sealed under another secret, or older
 * than the ten minutes a consent screen should take.
 */
export async function openFlow(sealed: string, secret: string, now: number): Promise<Flow | null> {
	try {
		const joined = unbase64url(sealed);
		if (joined.length <= NONCE_BYTES) return null;
		const plaintext = await crypto.subtle.decrypt(
			{ name: "AES-GCM", iv: joined.subarray(0, NONCE_BYTES) },
			await flowKey(secret),
			joined.subarray(NONCE_BYTES),
		);
		const flow = JSON.parse(new TextDecoder().decode(plaintext)) as Flow;
		if (typeof flow?.state !== "string" || typeof flow.verifier !== "string") return null;
		if (typeof flow.expiresAt !== "number" || flow.expiresAt < now) return null;
		return flow;
	} catch {
		return null;
	}
}

/**
 * Compare the state Google returned with the one the cookie kept.
 *
 * Length first, then every character, so how long the comparison takes says
 * nothing about how much of a guess was right.
 */
export function sameState(expected: string, received: string | null): boolean {
	if (!received || expected.length !== received.length) return false;
	let difference = 0;
	for (let i = 0; i < expected.length; i += 1) {
		difference |= expected.charCodeAt(i) ^ received.charCodeAt(i);
	}
	return difference === 0;
}

async function flowKey(secret: string): Promise<CryptoKey> {
	const material = await crypto.subtle.digest(
		"SHA-256",
		new TextEncoder().encode(`home-lab backups google flow\u0000${secret}`),
	);
	return crypto.subtle.importKey("raw", material, "AES-GCM", false, ["encrypt", "decrypt"]);
}

function base64url(bytes: Uint8Array): string {
	let binary = "";
	for (const byte of bytes) binary += String.fromCharCode(byte);
	return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function unbase64url(value: string): Uint8Array<ArrayBuffer> {
	const padded = value.replace(/-/g, "+").replace(/_/g, "/");
	const binary = atob(padded + "=".repeat((4 - (padded.length % 4)) % 4));
	const bytes = new Uint8Array(new ArrayBuffer(binary.length));
	for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i);
	return bytes;
}
