import type { NextAuthConfig } from "next-auth";
import type { JWT } from "next-auth/jwt";
import {
	CLIENT_ID,
	CLIENT_SECRET,
	ISSUER,
	PROVIDER_ID,
	PROVIDER_NAME,
	SCOPES,
} from "@/lib/auth/oidc-config";
import { renew } from "@/lib/auth/renew";
import { needsRefresh } from "@/lib/auth/token-set";

/**
 * Edge-safe Auth.js configuration, shared with the full `auth.ts`.
 *
 * The panel signs the operator in against eetr-auth and then keeps the resulting
 * access token, because the panel is a client of its own API and that API is an
 * OIDC resource server: every call carries the operator's bearer token. The
 * assistant agent will later carry exactly the same one.
 *
 * Sessions are JWTs sealed into an encrypted cookie (JWE, A256CBC-HS512, keyed
 * off AUTH_SECRET), so the tokens are opaque to the browser and any replica can
 * read them without shared storage.
 *
 * Authorization is membership, not roles: eetr-auth issues no role claim and
 * refuses `/authorize` unless the user is granted this client's environment. Any
 * operator who can obtain a token here is an operator here.
 */

/** True when sign-in is configured. Missing configuration is fatal, not a mode. */
export const authConfigured = Boolean(ISSUER && CLIENT_ID && CLIENT_SECRET);

export const authConfig: NextAuthConfig = {
	// The panel sits behind the cluster Gateway, so the request host is the
	// operator's hostname rather than the pod's.
	trustHost: true,
	session: { strategy: "jwt" },
	pages: { signIn: "/" },
	providers: [
		{
			id: PROVIDER_ID,
			name: PROVIDER_NAME,
			type: "oidc",
			issuer: ISSUER,
			clientId: CLIENT_ID,
			clientSecret: CLIENT_SECRET,
			// PKCE is mandatory and S256-only at eetr-auth, for confidential clients
			// too, so these are the defaults rather than an override — named here
			// because dropping one would still authenticate, just less safely.
			checks: ["pkce", "state", "nonce"],
			authorization: { params: { scope: SCOPES } },
		},
	],
	callbacks: {
		async jwt({ token, account, profile }) {
			// `account` is present only on the sign-in call, which is the one time the
			// tokens arrive from the provider.
			if (account) return adopt(token, account, profile);
			if (!token.refreshToken || !needsRefresh(token.expiresAt, nowSeconds())) {
				return token;
			}

			const outcome = await renew({
				accessToken: token.accessToken ?? "",
				refreshToken: token.refreshToken,
				expiresAt: token.expiresAt ?? 0,
			});

			// The refresh token is dropped along with the failure. eetr-auth rotates
			// them with reuse detection, so presenting a rejected one again is what
			// cascade-revokes the family — the session goes back through sign-in
			// rather than trying once more.
			if (!outcome.ok) return failed(token);

			return {
				...token,
				accessToken: outcome.tokens.accessToken,
				refreshToken: outcome.tokens.refreshToken,
				expiresAt: outcome.tokens.expiresAt,
				error: undefined,
			};
		},

		/**
		 * The session is server-only. There is no `SessionProvider` and no
		 * `useSession` in this app, and the access token below is why: handing this
		 * object to a client component would ship the operator's bearer token to the
		 * browser. Read it from server actions — see src/app/actions.
		 */
		session({ session, token }) {
			session.accessToken = token.accessToken;
			session.error = token.error;
			if (token.sub) session.user.id = token.sub;
			return session;
		},
	},
};

/** Epoch seconds. */
function nowSeconds(): number {
	return Math.floor(Date.now() / 1000);
}

/** The tokens and identity from a fresh sign-in. */
function adopt(token: JWT, account: Account, profile: unknown): JWT {
	const claims = (profile ?? {}) as Record<string, unknown>;
	return {
		...token,
		// The provider's own subject, not Auth.js's per-sign-in `token.sub`: the API
		// identifies the caller by the `sub` inside the access token, and the two
		// have to name the same person.
		sub: asString(claims.sub) ?? token.sub,
		accessToken: account.access_token,
		refreshToken: account.refresh_token,
		expiresAt: account.expires_at,
		error: undefined,
	};
}

/** A session whose tokens are gone and which must sign in again. */
function failed(token: JWT): JWT {
	return {
		...token,
		accessToken: undefined,
		refreshToken: undefined,
		expiresAt: undefined,
		error: "RefreshFailed",
	};
}

/** The part of Auth.js's `account` this app reads. */
interface Account {
	access_token?: string;
	refresh_token?: string;
	expires_at?: number;
}

/** Narrow an unknown claim to a non-empty string, or undefined. */
function asString(value: unknown): string | undefined {
	return typeof value === "string" && value !== "" ? value : undefined;
}
