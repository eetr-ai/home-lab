/**
 * ttyd's WebSocket protocol, as much of it as the console speaks.
 *
 * Every message is one command byte followed by its payload. The client's first
 * message is different: a JSON object carrying the terminal's size, which is what
 * makes ttyd spawn the process at all — until it arrives, nothing runs. The
 * `AuthToken` in it is for ttyd's own basic auth, which the console does not use;
 * the gate in front of it already decided.
 *
 * Pure, so the framing is tested rather than discovered against a live shell.
 */

/** The subprotocol ttyd insists on during the handshake. */
export const TTYD_PROTOCOL = "tty";

/** Client → server commands. */
const INPUT = "0";
const RESIZE = "1";

/** Server → client commands. */
const OUTPUT = "0".charCodeAt(0);
const SET_TITLE = "1".charCodeAt(0);
const SET_PREFERENCES = "2".charCodeAt(0);

export interface Size {
	columns: number;
	rows: number;
}

const encoder = new TextEncoder();

/** The opening message: the size the shell starts at. */
export function initMessage(size: Size): string {
	return JSON.stringify({ AuthToken: "", columns: size.columns, rows: size.rows });
}

/** Keystrokes, or pasted text. */
export function inputMessage(data: string): Uint8Array {
	const body = encoder.encode(data);
	const message = new Uint8Array(body.length + 1);
	message[0] = INPUT.charCodeAt(0);
	message.set(body, 1);
	return message;
}

/** The terminal changed size; the shell should too. */
export function resizeMessage(size: Size): Uint8Array {
	return encoder.encode(RESIZE + JSON.stringify({ columns: size.columns, rows: size.rows }));
}

export type ServerMessage =
	| { kind: "output"; data: Uint8Array }
	| { kind: "title"; title: string }
	| { kind: "preferences" }
	| { kind: "unknown" };

const decoder = new TextDecoder();

/**
 * One message from ttyd. Output stays bytes: a multi-byte character can be split
 * across two messages, and xterm reassembles bytes where a string decoded here
 * would already be broken.
 */
export function decodeServerMessage(message: ArrayBuffer | Uint8Array): ServerMessage {
	const bytes = message instanceof Uint8Array ? message : new Uint8Array(message);
	if (bytes.length === 0) return { kind: "unknown" };

	const payload = bytes.subarray(1);
	switch (bytes[0]) {
		case OUTPUT:
			return { kind: "output", data: payload };
		case SET_TITLE:
			return { kind: "title", title: decoder.decode(payload) };
		case SET_PREFERENCES:
			return { kind: "preferences" };
		default:
			return { kind: "unknown" };
	}
}

/**
 * The socket URL for a session. `base` may be a path on the panel's own origin
 * (the normal case: the Gateway routes it to the console) or a full ws(s) URL,
 * which is what local development points at a gate on another port.
 */
export function socketUrl(base: string, session: string, page: { protocol: string; host: string }): string {
	const scheme = page.protocol === "https:" ? "wss:" : "ws:";
	const url = base.startsWith("ws:") || base.startsWith("wss:")
		? new URL(base)
		: new URL(base, `${scheme}//${page.host}`);
	url.searchParams.set("session", session);
	return url.toString();
}
