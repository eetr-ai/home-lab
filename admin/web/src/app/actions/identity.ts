"use server";

import { withRead } from "./_auth";
import { whoami } from "@/lib/api/identity";
import type { ActionResult } from "@/lib/api/result";
import type { Whoami } from "@/lib/api/types";

/**
 * Ask the API who it thinks is calling.
 *
 * The panel's end-to-end check: it succeeds only if the operator's token was
 * stored, sent, verified against eetr-auth's keys, and read back.
 */
export async function describeCaller(): Promise<ActionResult<Whoami>> {
	return withRead(() => whoami());
}
