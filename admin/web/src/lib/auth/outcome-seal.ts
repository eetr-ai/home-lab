/**
 * Encrypting a refresh outcome before it is shared.
 *
 * The coordinated refresh in shared-refresh.ts has to publish its result so the
 * losers of the race can read it, and that result is a live access and refresh
 * token pair. Redis is password-protected and reachable only from a namespace
 * somebody labelled, but neither of those is a reason to put credentials in it in
 * the clear: the store is a different trust domain from the process, it is
 * readable by anything holding the password, and keys and values pass through
 * MONITOR and SLOWLOG.
 *
 * So what is stored is ciphertext only this deployment can open. The key comes
 * from AUTH_SECRET, which already seals the session cookie carrying the same
 * tokens — the same secret protecting the same values in the two places they
 * rest, rather than a second one to rotate and get wrong.
 *
 * AES-256-GCM: authenticated, so a value that was tampered with fails to open
 * rather than decrypting into something that is then trusted. The nonce is random
 * per value and travels with it. WebCrypto rather than node:crypto because it is
 * what runs in every Next.js runtime without a polyfill.
 */
import type { RefreshOutcome } from "./refresh";

const NONCE_BYTES = 12;

/**
 * Derive the content key from AUTH_SECRET.
 *
 * SHA-256 of the secret rather than the raw bytes, because AES-256-GCM needs
 * exactly 32 of them and AUTH_SECRET is an arbitrary string. This is not password
 * hashing — the input is already a high-entropy secret, so a KDF's work factor
 * would buy nothing and cost a round trip on every refresh.
 */
async function contentKey(secret: string): Promise<CryptoKey> {
	const material = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(secret));
	return crypto.subtle.importKey("raw", material, "AES-GCM", false, ["encrypt", "decrypt"]);
}

/** Seal an outcome for the shared store. */
export async function sealOutcome(outcome: RefreshOutcome, secret: string): Promise<string> {
	const key = await contentKey(secret);
	const nonce = crypto.getRandomValues(new Uint8Array(NONCE_BYTES));
	const plaintext = new TextEncoder().encode(JSON.stringify(outcome));
	const ciphertext = new Uint8Array(
		await crypto.subtle.encrypt({ name: "AES-GCM", iv: nonce }, key, plaintext),
	);

	const joined = new Uint8Array(nonce.length + ciphertext.length);
	joined.set(nonce);
	joined.set(ciphertext, nonce.length);
	return base64(joined);
}

/**
 * Open a sealed outcome, or null if it cannot be authenticated.
 *
 * Null rather than a throw for every failure mode — a truncated value, a
 * different AUTH_SECRET, something that is not base64 at all — because the caller
 * treats an unreadable value as a cache miss and recovers by doing the exchange
 * itself. A throw there would turn a rotated secret into every refresh failing
 * until the old values expired.
 */
export async function openOutcome(
	sealed: string,
	secret: string,
): Promise<RefreshOutcome | null> {
	try {
		const joined = unbase64(sealed);
		if (joined.length <= NONCE_BYTES) return null;

		const key = await contentKey(secret);
		const plaintext = await crypto.subtle.decrypt(
			{ name: "AES-GCM", iv: joined.subarray(0, NONCE_BYTES) },
			key,
			joined.subarray(NONCE_BYTES),
		);
		const parsed = JSON.parse(new TextDecoder().decode(plaintext)) as RefreshOutcome;

		// Shape-checked rather than trusted. This value came from a shared store, and
		// an outcome that is neither ok nor a failure would flow into the session as
		// undefined tokens.
		if (parsed?.ok === true && parsed.tokens?.accessToken) return parsed;
		if (parsed?.ok === false && typeof parsed.error === "string") return parsed;
		return null;
	} catch {
		return null;
	}
}

/**
 * The refresh token's key in the store.
 *
 * Hashed so the token itself is never a key. Keys are the part of a store that
 * leaks most readily — MONITOR streams them, SLOWLOG samples them, `KEYS *` lists
 * them — and a refresh token in one is a credential in all three.
 */
export async function digestToken(refreshToken: string): Promise<string> {
	const digest = await crypto.subtle.digest(
		"SHA-256",
		new TextEncoder().encode(refreshToken),
	);
	return base64(new Uint8Array(digest)).replace(/=+$/, "");
}

function base64(bytes: Uint8Array): string {
	let binary = "";
	for (const byte of bytes) binary += String.fromCharCode(byte);
	return btoa(binary);
}

/**
 * Typed as backed by a plain ArrayBuffer rather than the default
 * `ArrayBufferLike`, so the views taken of it below satisfy WebCrypto's
 * BufferSource. Without it `subarray` is a Uint8Array that TypeScript must assume
 * could sit on a SharedArrayBuffer, which crypto.subtle does not accept.
 */
function unbase64(value: string): Uint8Array<ArrayBuffer> {
	const binary = atob(value);
	const bytes = new Uint8Array(new ArrayBuffer(binary.length));
	for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i);
	return bytes;
}
