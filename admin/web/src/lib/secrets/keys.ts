/**
 * What Kubernetes will accept as a Secret data key.
 *
 * One module because three places need the identical rule — composing a database
 * credential's Secret, filling in a new Secret by hand, and choosing what a
 * rotation replaces — and a validation regex copied three times is a validation
 * regex that will disagree with itself.
 *
 * Checked in the browser as well as in the API so the message names the offending
 * key, rather than arriving from the API server as a sentence about field paths.
 */

/** Alphanumerics, '-', '_' and '.', which is the whole of what Kubernetes allows. */
const KEY_PATTERN = /^[A-Za-z0-9_.-]+$/;

/**
 * Kubernetes' bound on a data key. The key is a path segment when the Secret is
 * mounted as a volume, which is where the limit comes from.
 */
const MAX_KEY_LENGTH = 253;

/**
 * Why this is not a valid key, or null when it is.
 *
 * Three rules rather than one. The characters are the easy half; the other half
 * is that a key becomes a filename when the Secret is mounted as a volume, so
 * ".", ".." and anything starting with ".." are refused outright. apimachinery's
 * IsConfigMapKey does exactly this, and matching it is the point — a key it would
 * refuse comes back as a 500 about a field path, and for a credential install it
 * comes back after the role has already been created.
 */
export function secretKeyProblem(key: string): string | null {
	if (key.length === 0 || !KEY_PATTERN.test(key)) {
		return `"${key}" is not a valid Secret key.`;
	}
	if (key.length > MAX_KEY_LENGTH) {
		return `"${key}" is longer than the ${MAX_KEY_LENGTH} characters a Secret key may be.`;
	}
	// Spelled out rather than folded into the pattern, because the reason is not
	// about characters and the message should say so.
	if (key === "." || key === ".." || key.startsWith("..")) {
		return `"${key}" is a path Kubernetes reserves, so it cannot be a Secret key.`;
	}
	return null;
}

/** The same verdict, for the callers that only need a yes or no. */
export function isValidSecretKey(key: string): boolean {
	return secretKeyProblem(key) === null;
}
