import NextAuth from "next-auth";
import { authConfig } from "./auth.config";

/**
 * The full Auth.js instance. The config has no database adapter — JWT sessions
 * only — so it is edge-safe, and the same instance backs the route handlers, the
 * server actions, and the proxy. `AUTH_SECRET` is read from the environment.
 */
export const { handlers, auth, signIn, signOut } = NextAuth(authConfig);

export { authConfigured } from "./auth.config";
