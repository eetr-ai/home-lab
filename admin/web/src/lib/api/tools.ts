import { call } from "./http";
import type { ActionResult } from "./result";
import type { GeneratedValue } from "./types";

/**
 * The panel's small tools. Mirrors admin/api/internal/secretgen.
 *
 * One so far: minting a credential. It lives in the API rather than in the
 * browser because the assistant needs the same generator, and one implementation
 * of rejection sampling is better than two that agree until they do not.
 */
export async function generateSecretValue(
	shape: string,
	length: number,
): Promise<ActionResult<GeneratedValue>> {
	const query = new URLSearchParams({ shape, length: String(length) });
	return call<GeneratedValue>("GET", `/api/secret-values?${query}`);
}
