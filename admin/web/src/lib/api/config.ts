import "server-only";

import { auth } from "@/auth";

/**
 * How the app reaches the admin API, and with whose credential.
 *
 * Extracted from http.ts so the log-streaming route handler can share it. That
 * route cannot go through `call()`: it buffers a whole response and gives up
 * after twenty seconds, and a log tail is neither bounded nor finished in twenty
 * seconds. It still has to resolve the base URL and the operator's token exactly
 * the way every other call does, and two implementations of that would drift.
 *
 * `import "server-only"` is load-bearing here for the same reason it is in
 * http.ts: this reads the operator's bearer token, so importing it from a client
 * component has to be a build failure rather than a review someone might miss.
 */

/** The admin API's base URL with any trailing slash trimmed, or "" when unset. */
export function adminApiBaseUrl(): string {
	return (process.env.ADMIN_API_URL ?? "").replace(/\/+$/, "");
}

/**
 * The signed-in operator's bearer token, or the reason there is not one.
 *
 * A session whose refresh failed still looks signed in — it is not, and its
 * tokens are gone, so saying so here is more useful than letting the API answer
 * 401 with a message that cannot explain why.
 */
export async function bearerToken(): Promise<{ token: string } | { error: string }> {
	const session = await auth();
	if (session?.error === "RefreshFailed") {
		return { error: "the session expired and could not be renewed — sign in again" };
	}
	if (!session?.accessToken) {
		return { error: "the session carries no access token — sign in again" };
	}
	return { token: session.accessToken };
}
