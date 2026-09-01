import { isValidSecretKey } from "@/lib/secrets/keys";
import type { PutSecret, RotateSecret, SecretSummary } from "@/lib/api/types";

/**
 * The rules behind the two Secret forms, kept out of the components.
 *
 * A Secret is a list of key/value rows, and a list of rows is where the silent
 * mistakes live: two rows with the same key leave one value and it is whichever
 * was written last, and a row filled in on one side only produces a Secret that
 * exists and does not work. Both are invisible once the object is written, since
 * nothing reads a value back to check.
 *
 * So the drafts turn into a result rather than into a payload, and the caller
 * cannot forget to look. Same division of labour as lib/secrets/install-draft.ts.
 */

/** One key/value row, as the form holds it. */
export interface SecretRow {
	/** A stable key for React, since neither the name nor the position is one. */
	id: string;
	key: string;
	value: string;
}

export interface CreateDraft {
	name: string;
	rows: SecretRow[];
	/**
	 * Replace a Secret that is already there. Off unless asked for — overwriting
	 * one is how a running release loses the credential it was started with.
	 */
	overwrite: boolean;
}

export type CreatePlan =
	| { ok: true; name: string; request: PutSecret }
	| { ok: false; error: string };

export type RotatePlan = { ok: true; request: RotateSecret } | { ok: false; error: string };

/**
 * A name the API will accept for a Secret it is creating.
 *
 * The DNS *label* rule, not the subdomain one Kubernetes actually allows, and the
 * API is stricter here for the same reason: a Secret the panel writes is one
 * somebody has to type into a values file. Checked here so the message arrives
 * under the field instead of as a 400.
 */
const NAME_PATTERN = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;
const MAX_NAME_LENGTH = 63;

export function planCreate(draft: CreateDraft): CreatePlan {
	const name = draft.name.trim();
	if (!name) return { ok: false, error: "Name the Secret." };
	if (name.length > MAX_NAME_LENGTH || !NAME_PATTERN.test(name)) {
		return {
			ok: false,
			error: "A name is lowercase letters, digits and hyphens, starting and ending with one.",
		};
	}

	const data = collect(draft.rows);
	if (!data.ok) return data;
	if (Object.keys(data.data).length === 0) {
		return { ok: false, error: "A Secret needs at least one key." };
	}

	return { ok: true, name, request: { data: data.data, overwrite: draft.overwrite } };
}

/**
 * What a rotation sends: the chosen keys and their new values, and nothing else.
 *
 * A key that is not on the Secret is refused here as well as by the API, because
 * the message is better — "rotation replaces a value" is the rule, and it reads
 * as an explanation under the field rather than as a rejected request.
 */
export function planRotate(secret: SecretSummary, rows: SecretRow[]): RotatePlan {
	const chosen = rows.filter((row) => row.key);
	if (chosen.length === 0) {
		return { ok: false, error: "Choose at least one key to rotate." };
	}

	const unknown = chosen.find((row) => !secret.keys.includes(row.key));
	if (unknown) {
		return {
			ok: false,
			error: `"${unknown.key}" is not a key on this Secret. Rotation replaces a value; it does not add one.`,
		};
	}

	const data = collect(chosen);
	if (!data.ok) return data;
	return { ok: true, request: { data: data.data } };
}

/**
 * Rows to a payload, or to the reason there is not one.
 *
 * The duplicate check is the point. `Object.fromEntries` over two rows with the
 * same key silently keeps the last, so a Secret meant to hold a username and a
 * password can end up holding the password twice — and since nothing reads it
 * back, the first sign would be a workload that will not authenticate.
 */
function collect(
	rows: SecretRow[],
): { ok: true; data: Record<string, string> } | { ok: false; error: string } {
	const filled = rows.filter((row) => row.key || row.value);

	for (const row of filled) {
		if (!row.key) return { ok: false, error: `A value was given with no key to hold it.` };
		if (!isValidSecretKey(row.key)) {
			return { ok: false, error: `"${row.key}" is not a valid Secret key.` };
		}
		// An empty value is refused rather than stored. A Secret whose password key
		// holds "" starts the workload and fails to authenticate, and nothing about
		// that says the credential was never written.
		if (!row.value) return { ok: false, error: `"${row.key}" has no value.` };
	}

	const keys = filled.map((row) => row.key);
	const collision = keys.find((key, index) => keys.indexOf(key) !== index);
	if (collision) return { ok: false, error: `Two rows are both named "${collision}".` };

	return { ok: true, data: Object.fromEntries(filled.map((row) => [row.key, row.value])) };
}
