/**
 * Reading a replica count out of a text field.
 *
 * `Number("")` is 0 and `Number.isInteger(0)` is true, so the obvious expression
 * treats a cleared field as a deliberate "scale to zero". That is the one value
 * where getting it wrong takes a workload down, so the parsing lives here with
 * tests rather than inline in a component.
 *
 * The `min` and `max` attributes on a number input are hints. Nothing submits a
 * form here, so the browser blocks neither a typed -3 nor a typed 5000, and both
 * would otherwise reach the API.
 */

/**
 * The largest count the panel will send.
 *
 * Must match maxReplicas in admin/api/internal/kube/names.go — the API refuses
 * anything larger, and a UI that lets it be typed just turns a typo into a
 * round trip and an error banner.
 */
export const MAX_REPLICAS = 100;

/**
 * The count the field holds, or null when it does not hold one.
 *
 * Null for blank, for anything non-numeric, and for anything out of range —
 * every case where there is no number the operator meant.
 */
export function parseReplicas(raw: string): number | null {
	const trimmed = raw.trim();
	// Digits only: this rejects "", "-3", "1.5", "1e3", and " " in one test, each
	// of which Number() would otherwise turn into something plausible.
	if (!/^\d+$/.test(trimmed)) return null;

	const replicas = Number(trimmed);
	if (!Number.isInteger(replicas) || replicas > MAX_REPLICAS) return null;
	return replicas;
}
