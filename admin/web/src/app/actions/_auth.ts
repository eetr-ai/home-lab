import "server-only";

import { auth } from "@/auth";
import { permitsWrite, writeAllowlist } from "@/lib/auth/write-access";
import type { ActionResult } from "@/lib/api/result";

/**
 * The authorization gates every server action passes through.
 *
 * The action is the trust boundary: it authorizes, then delegates to the
 * auth-agnostic API client. Reads require a session; writes additionally require
 * the caller to be permitted by `ADMIN_WRITE_EMAILS` when that is set.
 *
 * A denied check is a result, never a throw — Next.js redacts a thrown server
 * action error in production, and "forbidden" is exactly the thing the operator
 * needs to read.
 */

/** Run `fn` for any signed-in operator. */
export async function withRead<T>(fn: () => Promise<ActionResult<T>>): Promise<ActionResult<T>> {
	const denied = await denyUnlessSignedIn();
	return denied ?? fn();
}

/** Run `fn` only for an operator permitted to make changes. */
export async function withWrite<T>(fn: () => Promise<ActionResult<T>>): Promise<ActionResult<T>> {
	const denied = await denyUnlessSignedIn();
	if (denied) return denied;

	const session = await auth();
	if (!permitsWrite(writeAllowlist(process.env.ADMIN_WRITE_EMAILS), session?.user?.email)) {
		return { ok: false, error: "this account may view the panel but not change anything" };
	}
	return fn();
}

/**
 * The refusal to report, or undefined when the caller is signed in.
 *
 * Exported because the log-streaming route handler needs the same rule and is not
 * shaped like an action — it returns a Response, not an ActionResult, so it
 * cannot go through withRead. One definition of "signed in" rather than two that
 * will drift.
 */
export async function denyUnlessSignedIn(): Promise<{ ok: false; error: string } | undefined> {
	const session = await auth();
	if (!session?.user) return { ok: false, error: "not signed in" };
	// A session whose refresh failed still looks signed in. It is not: its tokens
	// are gone and every API call would come back 401 with a less useful message.
	if (session.error === "RefreshFailed") {
		return { ok: false, error: "the session expired and could not be renewed — sign in again" };
	}
	return undefined;
}
