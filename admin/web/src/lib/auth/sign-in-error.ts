/**
 * Turning an Auth.js sign-in error code into something an operator can act on.
 *
 * Auth.js reports failures by redirecting back to the sign-in page with an
 * `?error=` code. The codes are accurate and useless — `AccessDenied` does not
 * say that eetr-auth refuses `/authorize` for a user who has not been granted
 * this client's environment, which is far and away the most likely reason to see
 * it here and is fixed in eetr-auth rather than in this panel.
 */

const MESSAGES: Record<string, string> = {
	AccessDenied:
		"eetr-auth refused the sign-in. Grant this account the admin panel client's environment in eetr-auth — being an eetr-auth administrator is not enough on its own.",
	Configuration:
		"The panel's OIDC configuration is incomplete or does not match the client registered in eetr-auth.",
	OAuthCallbackError:
		"eetr-auth rejected the callback. Check that this panel's redirect URI is registered exactly, including the scheme and any port.",
	OAuthSignInError:
		"The panel could not start the sign-in. eetr-auth may be unreachable from here.",
	Verification: "That sign-in link is no longer valid. Start again.",
};

/** The message for a code, or a general one. Returns null when nothing failed. */
export function signInErrorMessage(code: string | undefined): string | null {
	if (!code) return null;
	return MESSAGES[code] ?? "Sign-in failed. Try again, and check the panel's logs if it persists.";
}
