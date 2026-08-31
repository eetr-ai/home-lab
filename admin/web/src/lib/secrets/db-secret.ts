/**
 * Composing the Secret that carries a database credential into a namespace.
 *
 * This is a pure function on purpose. The panel creates a role and then writes a
 * Secret holding what it just issued, and which keys that Secret has is decided
 * by whatever chart is going to read it — `existingSecretPasswordKey` and its
 * neighbours differ from chart to chart. So the key names are the operator's to
 * choose, which means they can collide, and a collision is silent: two keys with
 * the same name in an object literal leave one value, and the one that survives
 * is whichever was written last. That would produce a Secret whose password key
 * holds the username.
 *
 * Hence a result rather than a map. The caller cannot forget to check it.
 */

/** What the operator chose. Empty key names mean "do not include this". */
export interface SecretLayout {
	usernameKey: string;
	passwordKey: string;
	databaseKey: string;
}

/** The credential to carry. `database` is absent for a role with no database. */
export interface Credential {
	username: string;
	password: string;
	database?: string;
}

export type SecretDataResult =
	| { ok: true; data: Record<string, string> }
	| { ok: false; error: string };

/** The keys most charts expect, which is what the form starts from. */
export const DEFAULT_LAYOUT: SecretLayout = {
	usernameKey: "username",
	passwordKey: "password",
	databaseKey: "",
};

/**
 * Every character Kubernetes permits in a Secret data key. Checked here so the
 * message names the key, rather than arriving from the API server as a sentence
 * about field paths.
 */
const KEY_PATTERN = /^[A-Za-z0-9_.-]+$/;

export function credentialSecretData(
	credential: Credential,
	layout: SecretLayout,
): SecretDataResult {
	const entries: [string, string][] = [];

	if (layout.usernameKey) entries.push([layout.usernameKey, credential.username]);
	if (layout.passwordKey) entries.push([layout.passwordKey, credential.password]);
	if (layout.databaseKey && credential.database) {
		entries.push([layout.databaseKey, credential.database]);
	}

	if (!entries.some(([key]) => key === layout.passwordKey) || !layout.passwordKey) {
		return { ok: false, error: "The Secret needs a key to hold the password." };
	}

	for (const [key, value] of entries) {
		if (!KEY_PATTERN.test(key)) {
			return { ok: false, error: `"${key}" is not a valid Secret key.` };
		}
		if (!value) {
			return { ok: false, error: `"${key}" would have no value.` };
		}
	}

	const names = entries.map(([key]) => key);
	const collision = names.find((key, index) => names.indexOf(key) !== index);
	if (collision) {
		return { ok: false, error: `Two fields are both named "${collision}".` };
	}

	return { ok: true, data: Object.fromEntries(entries) };
}
