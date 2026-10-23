/**
 * The request the gate sends on to ttyd once a socket is admitted.
 *
 * The gate does not speak WebSocket; it forwards the upgrade request and then
 * splices bytes. So all it has to get right is the head: the path ttyd serves its
 * socket on, the session as ttyd's `arg`, and the handshake headers unchanged.
 *
 * Two headers are deliberately left behind. The cookie carries the operator's
 * sealed tokens and the shell has no use for them. The origin is the browser's,
 * which ttyd would only compare with itself. Pure, so the rewrite is tested.
 */

/** The handshake headers a WebSocket upgrade cannot do without. */
const FORWARDED = [
	"upgrade",
	"connection",
	"sec-websocket-key",
	"sec-websocket-version",
	"sec-websocket-protocol",
	"sec-websocket-extensions",
] as const;

/** ttyd's socket, behind its default base path. */
export const TTYD_SOCKET_PATH = "/ws";

/** The HTTP/1.1 request head for ttyd, ending in the blank line. */
export function upstreamHead(
	headers: Record<string, string | string[] | undefined>,
	session: string,
	host: string,
): string {
	const lines = [
		`GET ${TTYD_SOCKET_PATH}?arg=${encodeURIComponent(session)} HTTP/1.1`,
		`Host: ${host}`,
	];
	for (const name of FORWARDED) {
		const value = headers[name];
		if (value === undefined) continue;
		lines.push(`${name}: ${Array.isArray(value) ? value.join(", ") : value}`);
	}
	return `${lines.join("\r\n")}\r\n\r\n`;
}
