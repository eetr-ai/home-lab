import type { RedisText } from "@/lib/api/redis-types";

/**
 * Turning Redis's bytes into something a URL or a page can hold.
 *
 * Redis keys are bytes, not strings. The API sends a key back as text when it is
 * valid UTF-8 and as base64 when it is not, and the two have to be turned back
 * into the same bytes on the way in again — otherwise a binary key can be listed
 * and never opened.
 */

/**
 * A key as one URL path segment.
 *
 * Text goes through encodeURIComponent, which percent-encodes its UTF-8. Base64
 * is decoded to the raw bytes and every byte percent-encoded, which is the one
 * form the API's router decodes back to exactly those bytes — encodeURIComponent
 * on a string holding them would encode each as UTF-8 first and send different
 * bytes.
 */
export function keySegment(key: RedisText): string {
	if (key.encoding === "utf8") return encodeURIComponent(key.value);

	let ret = "";
	for (const byte of base64Bytes(key.value)) {
		ret += `%${byte.toString(16).toUpperCase().padStart(2, "0")}`;
	}
	return ret;
}

/** A stable identity for a key, for React keys and "which row is confirming". */
export function keyId(key: RedisText): string {
	return `${key.encoding}:${key.value}`;
}

/**
 * A value for display. Base64 is shown as base64, marked, rather than decoded
 * into replacement characters that would look like text and copy as garbage.
 */
export function displayText(text: RedisText): string {
	return text.encoding === "utf8" ? text.value : `base64:${text.value}`;
}

function base64Bytes(value: string): Uint8Array {
	const binary = atob(value);
	const ret = new Uint8Array(binary.length);
	for (let i = 0; i < binary.length; i++) ret[i] = binary.charCodeAt(i);
	return ret;
}

/**
 * A TTL in words.
 *
 * -1 is Redis's "never expires", which is the common case for a lock that has
 * gone wrong and the reason anyone looks: a lock with no TTL is one nothing will
 * ever release.
 */
export function formatTtl(millis: number): string {
	if (millis < 0) return "never";
	if (millis < 1000) return `${millis}ms`;

	const seconds = Math.floor(millis / 1000);
	if (seconds < 60) return `${seconds}s`;
	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) return `${minutes}m ${seconds % 60}s`;
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours}h ${minutes % 60}m`;
	return `${Math.floor(hours / 24)}d ${hours % 24}h`;
}
