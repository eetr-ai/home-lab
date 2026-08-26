/**
 * What this app adds to Auth.js's session and JWT.
 *
 * The access token appears on both because the panel calls its own API as the
 * signed-in operator. None of it may be read from a client component — see the
 * session callback in auth.config.ts.
 */
import type { DefaultSession } from "next-auth";

declare module "next-auth" {
	interface Session {
		/** The operator's eetr-auth access token. Server-only. */
		accessToken?: string;
		/** Set when a refresh failed, meaning the session has to sign in again. */
		error?: "RefreshFailed";
		user: { id?: string } & DefaultSession["user"];
	}
}

declare module "next-auth/jwt" {
	interface JWT {
		accessToken?: string;
		refreshToken?: string;
		/** Epoch seconds at which `accessToken` stops being usable. */
		expiresAt?: number;
		error?: "RefreshFailed";
	}
}
