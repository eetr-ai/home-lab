import { call } from "./http";
import type { ActionResult } from "./result";
import type { Whoami } from "./types";

/**
 * Who the API thinks is calling.
 *
 * Worth more than it looks: it is the one call that proves the whole chain — the
 * operator's eetr-auth token was stored, sent as a bearer credential, verified
 * against the issuer's keys, and its subject read back. When the panel cannot
 * reach the API, this is what says so first.
 */
export function whoami(): Promise<ActionResult<Whoami>> {
	return call<Whoami>("GET", "/api/whoami");
}
