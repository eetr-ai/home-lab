import "server-only";

import { RestClient } from "@eetr/ts-rest-utils";
import { auth } from "@/auth";
import { errorMessage } from "./errors";
import type { ActionResult } from "./result";

/**
 * The transport every admin-api call goes through, and the only code in the app
 * that touches HTTP.
 *
 *     server action (auth) → a typed client module → call() → RestClient → fetch
 *
 * It is one `RestClient` from `@eetr/ts-rest-utils`, configured once. The
 * credential is the signed-in operator's own eetr-auth access token, supplied
 * through the library's `authProvider` hook — which means authentication is a
 * property of the client rather than something each call has to remember, and
 * the assistant agent will reach the same API the same way with the token of
 * whoever is typing.
 *
 * `import "server-only"` is load-bearing. This module can read the operator's
 * bearer token, so importing it from a client component has to be a build
 * failure rather than a code review someone might miss.
 */

/** How long a single call may take. Comfortably under any browser's patience. */
const TIMEOUT_MS = 20_000;

const client = new RestClient({
	baseUrl: baseUrl(),
	// Called once per attempt, so a token renewed by the proxy since this request
	// began is the one that gets sent.
	authProvider: async () => {
		const session = await auth();
		const token = session?.accessToken;
		return token ? { Authorization: `Bearer ${token}` } : undefined;
	},
	timeoutMs: TIMEOUT_MS,
	// No retry policy on purpose. Several of these operations create and drop
	// databases and roles; a transparent second attempt at one of those is a way
	// to do it twice.
});

/** The admin API's base URL with any trailing slash trimmed, or "" when unset. */
function baseUrl(): string {
	return (process.env.ADMIN_API_URL ?? "").replace(/\/+$/, "");
}

/**
 * Issue one admin-api request.
 *
 * Internal to `src/lib/api`: the public surface is the named domain functions in
 * the sibling modules, never a verb and a path. Never throws — a network failure,
 * a timeout, and a 500 all arrive as the same kind of value.
 */
export async function call<T>(
	method: "GET" | "POST" | "DELETE",
	path: string,
	body?: unknown,
): Promise<ActionResult<T>> {
	if (!baseUrl()) {
		return { ok: false, error: "the admin API is not configured (ADMIN_API_URL is unset)" };
	}

	const session = await auth();
	if (session?.error === "RefreshFailed") {
		return { ok: false, error: "the session expired and could not be renewed — sign in again" };
	}
	if (!session?.accessToken) {
		return { ok: false, error: "the session carries no access token — sign in again" };
	}

	try {
		const response = await client.request<T>(path, { method, body });
		if (response.ok) return { ok: true, data: response.body };
		return { ok: false, error: errorMessage(response.body, response.status) };
	} catch (err) {
		// The library throws only when there is no response to hand back: a network
		// failure, an expired deadline, a body that claimed to be JSON and was not.
		return { ok: false, error: `the admin API is unreachable: ${(err as Error).message}` };
	}
}

/** Encode one path segment. Names may contain characters a URL would eat. */
export const seg = encodeURIComponent;
