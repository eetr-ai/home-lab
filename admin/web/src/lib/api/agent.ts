import "server-only";

/**
 * How this app reaches the agent.
 *
 * Its own module rather than a constant in the route handler, for the same reason
 * `config.ts` is one: two route handlers need it, and two definitions of "where
 * the agent is" would drift. It is deliberately *not* in `config.ts` — that one is
 * about the admin API, and the agent is a different service with a different
 * address and a different reason to be absent.
 *
 * `import "server-only"` because an address the browser could read is an address
 * the browser could call, and this one has no authentication of its own: the
 * agent is a ClusterIP with no route, and the panel attaching the operator's token
 * is the whole of its access control.
 */

/** The agent's base URL with any trailing slash trimmed, or "" when unset. */
export function agentBaseUrl(): string {
	return (process.env.AGENT_URL ?? "").replace(/\/+$/, "");
}
