import type { EditSecret, SecretSummary } from "@/lib/api/types";
import { collect, type SecretFormat, type SecretRow } from "./secret-draft";

/**
 * The rules behind editing a Secret that already exists, kept out of the panel.
 *
 * Each key the Secret has is kept, replaced, or removed; new keys are typed in
 * beside them. The API is told which of those each key is, and refuses one in the
 * wrong list — so these rules are checked here too, where the message can name the
 * row, rather than arriving as a rejected request.
 */

/** What is happening to one key the Secret already has. */
export type KeyAction = "keep" | "replace" | "remove";

export interface ExistingKeyDraft {
	action: KeyAction;
	/** The new value, used only while the action is "replace". */
	value: string;
	format: SecretFormat;
}

export interface EditDraft {
	/** Keyed by the Secret's key. A key with no entry is kept. */
	existing: Record<string, ExistingKeyDraft>;
	/** Keys being added, as free-text rows. */
	added: SecretRow[];
}

export type EditPlan = { ok: true; request: EditSecret } | { ok: false; error: string };

export function emptyEdit(): EditDraft {
	return { existing: {}, added: [] };
}

/** The action a key is under, "keep" when nothing has been chosen for it. */
export function actionFor(draft: EditDraft, key: string): KeyAction {
	return draft.existing[key]?.action ?? "keep";
}

/**
 * The draft with one existing key moved to a new action, its value reset.
 *
 * Built with computed keys rather than assigned through `existing[key]`: a key
 * is whatever the Secret holds, and assigning `__proto__` on a plain object
 * sets its prototype instead of adding an entry, which the next copy then drops.
 */
export function withAction(draft: EditDraft, key: string, action: KeyAction): EditDraft {
	// Object.fromEntries defines own properties, so it is safe for any key too.
	const rest = Object.fromEntries(Object.entries(draft.existing).filter(([held]) => held !== key));
	const existing: Record<string, ExistingKeyDraft> =
		action === "keep" ? rest : { ...rest, [key]: { action, value: "", format: "password" } };
	return { ...draft, existing };
}

/** The draft with new values for keys already being replaced. Same care as withAction. */
export function withReplacedValues(draft: EditDraft, rows: SecretRow[]): EditDraft {
	let existing = draft.existing;
	for (const row of rows) {
		existing = { ...existing, [row.key]: { action: "replace", value: row.value, format: row.format } };
	}
	return { ...draft, existing };
}

/**
 * Whether there is anything to lose by closing the panel.
 *
 * An added row counts once anything is typed into it; an untouched blank row is
 * not a change somebody would miss.
 */
export function isEditDirty(draft: EditDraft): boolean {
	return (
		Object.keys(draft.existing).length > 0 || draft.added.some((row) => row.key || row.value)
	);
}

/**
 * What an edit sends: the keys being replaced, added and removed, nothing else.
 *
 * Replaced rows are ordered by the Secret's own keys so the error a check returns
 * is the first one on screen, not the first one ticked.
 */
export function planEdit(secret: SecretSummary, draft: EditDraft): EditPlan {
	const replaceRows: SecretRow[] = secret.keys
		.filter((key) => actionFor(draft, key) === "replace")
		.map((key) => ({ id: key, key, ...draft.existing[key] }));
	const remove = secret.keys.filter((key) => actionFor(draft, key) === "remove");

	// A replaced key with a blank value is a mistake to report, not a row to drop:
	// collect() would skip it as unfilled, and the operator would believe it rotated.
	const blank = replaceRows.find((row) => !row.value);
	if (blank) return { ok: false, error: `"${blank.key}" has no new value.` };
	const replace = collect(replaceRows);
	if (!replace.ok) return replace;

	const add = collect(draft.added);
	if (!add.ok) return add;
	// Adding is not a quiet overwrite, and the API would refuse it. Removing a key
	// and adding it back under the same name is a replace, and is said as one.
	const clash = Object.keys(add.data).find((key) => secret.keys.includes(key));
	if (clash) {
		return {
			ok: false,
			error: `This Secret already has "${clash}". Replace its value instead of adding it.`,
		};
	}

	const changes = replaceRows.length + remove.length + Object.keys(add.data).length;
	if (changes === 0) return { ok: false, error: "Nothing to change." };

	if (remove.length === secret.keys.length && Object.keys(add.data).length === 0) {
		return {
			ok: false,
			error: "That would remove every key. Delete the Secret instead.",
		};
	}

	const request: EditSecret = {};
	if (replaceRows.length > 0) request.replace = replace.data;
	if (Object.keys(add.data).length > 0) request.add = add.data;
	if (remove.length > 0) request.remove = remove;
	return { ok: true, request };
}
