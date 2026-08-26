import { handlers } from "@/auth";

/**
 * Auth.js's own endpoints: /api/auth/signin, /callback/eetr, /session, /signout.
 *
 * A route handler rather than a server action because the identity provider
 * redirects the browser here by URL — one of the few cases where an action
 * genuinely cannot serve. The callback path registered with eetr-auth is
 * `{AUTH_URL}/api/auth/callback/eetr`, and eetr-auth matches it exactly.
 */
export const { GET, POST } = handlers;
