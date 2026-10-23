import { getToken } from "@auth/core/jwt";
import { permitsWrite } from "@/lib/auth/write-access";

/**
 * Whether an upgrade request may have a shell.
 *
 * The console is a cluster-admin shell, so the rule is the strictest one the panel
 * has: a live session, on the write allowlist, from the panel's own origin. The
 * session is read straight from the Auth.js cookie — the same sealed JWT the panel
 * reads, decrypted with the same AUTH_SECRET — because a raw socket upgrade never
 * passes through Next and so cannot ask it.
 *
 * The origin check is the part a cookie makes necessary. A browser attaches the
 * panel's cookie to a WebSocket opened from ANY page, and the same-origin policy
 * does not apply to sockets; without it, a link in an email could open a shell
 * with the operator's session. Pure apart from the decryption, so the rule is
 * tested with cookies minted in the test.
 */

export interface GateConfig {
	/** AUTH_SECRET, exactly as the panel has it. */
	secret: string;
	/** The panel's public URL (AUTH_URL). Its origin is the only one admitted. */
	panelUrl: string;
	/** The parsed ADMIN_WRITE_EMAILS. Empty admits every signed-in operator. */
	allowlist: string[];
}

export type Verdict =
	| { ok: true; email: string | null }
	| { ok: false; status: 401 | 403; reason: string };

/** Decide whether `headers` may open a console. */
export async function authorize(
	headers: Record<string, string | string[] | undefined>,
	config: GateConfig,
): Promise<Verdict> {
	const panel = new URL(config.panelUrl);
	if (header(headers, "origin") !== panel.origin) {
		return { ok: false, status: 403, reason: "the console only opens from the panel itself" };
	}

	// Only the cookie is handed over. getToken also accepts a bearer header, and a
	// browser socket cannot send one — so anything that does is not the panel.
	const cookie = header(headers, "cookie") ?? "";
	const token = await getToken({
		req: { headers: { cookie } },
		secret: config.secret,
		// Auth.js names the cookie `__Secure-…` over https, and the salt follows the
		// name, so getting this wrong decrypts nothing rather than the wrong thing.
		secureCookie: panel.protocol === "https:",
	}).catch(() => null);

	if (!token) return { ok: false, status: 401, reason: "sign in to the panel first" };
	if (token.error) {
		return { ok: false, status: 401, reason: "your session expired; sign in again" };
	}

	const email = typeof token.email === "string" ? token.email : null;
	if (!permitsWrite(config.allowlist, email)) {
		return { ok: false, status: 403, reason: "the console needs write access to the panel" };
	}
	return { ok: true, email };
}

/** One header's value, the first when Node collected several. */
function header(
	headers: Record<string, string | string[] | undefined>,
	name: string,
): string | undefined {
	const value = headers[name];
	return Array.isArray(value) ? value[0] : value;
}
