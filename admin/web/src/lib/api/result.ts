/**
 * The value every call across the server-action boundary carries.
 *
 * A discriminated result rather than an exception, because Next.js redacts the
 * message of an error thrown inside a server action in production: the operator
 * would see "an error occurred" and the logs would hold nothing more. Branching
 * on `ok` keeps the reason.
 */
export type ActionResult<T> = { ok: true; data: T } | { ok: false; error: string };

/**
 * Turn a result back into value-or-throw, for a client component that would
 * rather use try/catch. Call it at the last moment, in the browser — never on the
 * way out of an action.
 */
export function unwrap<T>(result: ActionResult<T>): T {
	if (result.ok) return result.data;
	throw new Error(result.error);
}
