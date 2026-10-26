/**
 * How the OAuth callback tells the backups page what went wrong.
 *
 * The callback redirects with a code, never with words: anyone can write a link to
 * `/backups?driveError=...`, so whatever the page shows for it has to come from
 * this table. An unknown code shows nothing, and the detail behind a failed
 * connect stays in the server's log rather than travelling in the URL.
 */
const MESSAGES = {
	signed_out: "Sign in again, then connect Google Drive.",
	not_allowed: "This account may view the panel but not change anything.",
	expired: "The connection attempt expired or did not start here; try again.",
	denied: "Google Drive was not connected: access was not granted.",
	google_refused: "Google refused the connection.",
	no_code: "Google returned no authorization code; try again.",
	connect_failed: "Connecting Google Drive failed; the panel's log has the reason.",
} as const;

export type ConnectError = keyof typeof MESSAGES;

/** The sentence for a code from the callback, or null for anything it did not send. */
export function connectErrorMessage(code: string | undefined): string | null {
	if (!code || !Object.hasOwn(MESSAGES, code)) return null;
	return MESSAGES[code as ConnectError];
}

/**
 * The code for Google's own `error` parameter: access_denied is the operator
 * saying no, everything else is Google refusing before it asked them.
 */
export function googleRefusal(error: string): ConnectError {
	return error === "access_denied" ? "denied" : "google_refused";
}
