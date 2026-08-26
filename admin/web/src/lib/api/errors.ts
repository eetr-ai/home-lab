/**
 * Reading a failure out of an admin-api response.
 *
 * The API answers every failure with `{ error, message }` — a stable code and a
 * human-readable explanation that never carries internal detail (see
 * admin/api/internal/http/respond.go). This is the only place that knows that
 * shape, and it is pure so the parsing can be tested without a server.
 */

/** Statuses the panel explains rather than repeating verbatim. */
const EXPLAINED: Record<number, string> = {
	401: "the session is no longer accepted by the API — sign in again",
	403: "this account is not permitted to perform that operation",
	404: "that section is not enabled on this deployment",
	502: "the admin API could not reach the service it manages",
	503: "the admin API could not reach the service it manages",
};

/**
 * The message to show for a failed response.
 *
 * The body wins when it carries one, because the API says more than a status
 * code can. `error` is the fallback rather than the first choice: it is a code
 * meant for branching, and reading `invalid_request` is worse than reading the
 * sentence beside it.
 */
export function errorMessage(body: unknown, status: number): string {
	const fromBody = messageIn(body);
	if (fromBody) return fromBody;
	const explained = EXPLAINED[status];
	if (explained) return explained;
	return `the admin API returned ${status}`;
}

/** Pull `message`, then `error`, out of a parsed body that may be anything. */
function messageIn(body: unknown): string | undefined {
	// `res.json()` resolves to whatever decoded — null, a number, an array — so
	// reading a property off it directly would throw on a literal null body, and
	// nothing in this path is allowed to throw.
	if (body === null || typeof body !== "object") return undefined;
	const fields = body as { message?: unknown; error?: unknown };
	if (typeof fields.message === "string" && fields.message !== "") return fields.message;
	if (typeof fields.error === "string" && fields.error !== "") return fields.error;
	return undefined;
}
