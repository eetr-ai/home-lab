"use server";

import { signIn, signOut } from "@/auth";
import { PROVIDER_ID } from "@/lib/auth/oidc-config";

/**
 * Sign-in and sign-out, as server actions so a plain `<form action={…}>` drives
 * them and the page needs no client-side session at all.
 *
 * Both redirect, which in a server action means throwing a framework redirect —
 * so neither returns, and neither is an `ActionResult`.
 */

/** Begin the authorization-code flow, returning to `callbackUrl` afterwards. */
export async function startSignIn(formData: FormData): Promise<void> {
	const requested = formData.get("callbackUrl");
	// Only a same-site path is accepted. An absolute URL from the query string is
	// how an open redirect gets built, and this value reaches the browser as a
	// Location header.
	const callbackUrl =
		typeof requested === "string" && requested.startsWith("/") && !requested.startsWith("//")
			? requested
			: "/overview";
	await signIn(PROVIDER_ID, { redirectTo: callbackUrl });
}

/**
 * End the local session.
 *
 * Local only: eetr-auth publishes no `end_session_endpoint`, so there is no
 * RP-initiated logout to follow it with. Signing back in will not prompt for
 * credentials while the provider's own session is still alive.
 */
export async function endSession(): Promise<void> {
	await signOut({ redirectTo: "/" });
}
