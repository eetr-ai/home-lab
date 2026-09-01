/**
 * What the generator popover offers, and nothing about how a value is made.
 *
 * The generator itself is one implementation, in Go:
 * admin/api/internal/secretgen, reached through a server action. It lives there
 * because the assistant needs the same generator, and two copies of rejection
 * sampling and two copies of the alphabet would drift.
 *
 * This module is the labels, the hints and the bounds the form renders. The
 * shapes it names are the strings the API takes.
 */

export type Preset = "password" | "alphanumeric" | "hex" | "base64";

export interface PresetInfo {
	id: Preset;
	label: string;
	/** What this is for, shown under the choice. */
	hint: string;
	/** Whether length is the operator's to choose, or fixed by the shape. */
	sized: boolean;
}

/**
 * The four shapes worth offering, and what each is for.
 *
 * `base64` is 32 random bytes, which is what `npx auth secret` produces and what
 * AUTH_SECRET wants. It has no length because "32 bytes" is the requirement —
 * offering a slider there would invite somebody to pick 8.
 */
export const PRESETS: PresetInfo[] = [
	{
		id: "password",
		label: "Password",
		hint: "Letters, digits and symbols. For a database role or an application login.",
		sized: true,
	},
	{
		id: "alphanumeric",
		label: "Alphanumeric",
		hint: "Letters and digits only. For a value that will be pasted into a connection string or a shell.",
		sized: true,
	},
	{
		id: "hex",
		label: "Hex (32 bytes)",
		hint: "256 bits, hex encoded. For an API token or a signing key.",
		sized: false,
	},
	{
		id: "base64",
		label: "Base64 (32 bytes)",
		hint: "256 bits, base64 encoded. The AUTH_SECRET shape — what `npx auth secret` gives you.",
		sized: false,
	},
];

/**
 * The bounds, restated here so a mistyped length is a message under the field
 * rather than a round trip that comes back 400. The API enforces them; this is
 * only the nicer version of the same refusal.
 */
export const DEFAULT_LENGTH = 24;
export const MIN_LENGTH = 12;
export const MAX_LENGTH = 128;

/** Whether a length is one the API will accept, checked before asking it. */
export function isValidLength(length: number): boolean {
	return Number.isInteger(length) && length >= MIN_LENGTH && length <= MAX_LENGTH;
}
