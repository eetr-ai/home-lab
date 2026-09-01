"use server";

import { withRead } from "./_auth";
import * as tools from "@/lib/api/tools";
import type { ActionResult } from "@/lib/api/result";
import type { GeneratedValue } from "@/lib/api/types";

/**
 * Mint a credential.
 *
 * `withRead`, not `withWrite`: it changes nothing and touches nothing. An
 * operator who may view the panel but not change it can still be handed a value
 * to put somewhere else, and gating it behind the write allowlist would refuse
 * that for no benefit — the guard that matters is on installing the Secret, which
 * is a separate call and is a write.
 *
 * No `revalidatePath`, because nothing on any page has changed.
 */
export async function generateSecretValue(
	shape: string,
	length: number,
): Promise<ActionResult<GeneratedValue>> {
	return withRead(() => tools.generateSecretValue(shape, length));
}
