import { createServer, type IncomingMessage, STATUS_CODES } from "node:http";
import { connect, type Socket } from "node:net";
import { writeAllowlist } from "@/lib/auth/write-access";
import { authorize, type GateConfig } from "./authorize";
import { sessionName } from "@/lib/console/session-name";
import { upstreamHead } from "./upstream";

/**
 * The console gate: the only way into the console's shell.
 *
 * It runs beside ttyd in the console pod. ttyd listens on loopback, so a socket
 * reaches it through this process or not at all. The gate admits an upgrade
 * request at /console/socket when `authorize` says so, then forwards it to ttyd
 * and splices the two connections together until either side hangs up.
 *
 * It is plain node:http rather than a route handler because Next cannot accept a
 * socket upgrade without a custom server, and the panel's image would rather not
 * become one. It ships in that image anyway (bundled to console-gate.mjs) so the
 * cookie is decrypted by the same Auth.js code the panel signs it with.
 */

const SOCKET_PATH = "/console/socket";

const config: GateConfig = {
	secret: required("AUTH_SECRET"),
	panelUrl: required("AUTH_URL"),
	allowlist: writeAllowlist(process.env.ADMIN_WRITE_EMAILS),
};
const port = Number(process.env.PORT ?? 8080);
const ttydHost = process.env.CONSOLE_TTYD_HOST ?? "127.0.0.1";
const ttydPort = Number(process.env.CONSOLE_TTYD_PORT ?? 7681);

const server = createServer((req, res) => {
	// Probes only. Everything worth having arrives as an upgrade.
	const ok = req.method === "GET" && req.url === "/healthz";
	res.writeHead(ok ? 200 : 404, { "Content-Type": "text/plain" }).end(ok ? "ok\n" : "not found\n");
});

server.on("upgrade", (req: IncomingMessage, client: Socket, head: Buffer) => {
	void admit(req, client, head).catch((err: unknown) => {
		console.error("console gate: upgrade failed:", err);
		refuse(client, 502, "the console could not be reached");
	});
});

async function admit(req: IncomingMessage, client: Socket, head: Buffer): Promise<void> {
	const url = new URL(req.url ?? "/", "http://gate.invalid");
	if (url.pathname !== SOCKET_PATH) return refuse(client, 404, "not found");

	const session = sessionName(url.searchParams.get("session"));
	if (!session) return refuse(client, 400, "a session name is letters, digits, - and _");

	const verdict = await authorize(req.headers, config);
	if (!verdict.ok) return refuse(client, verdict.status, verdict.reason);

	let spliced = false;
	const shell = connect(ttydPort, ttydHost, () => {
		spliced = true;
		shell.write(upstreamHead(req.headers, session, `${ttydHost}:${ttydPort}`));
		// Bytes the client sent after its head belong to the socket, not to us.
		if (head.length > 0) shell.write(head);
		shell.pipe(client);
		client.pipe(shell);
	});
	console.log(`console gate: ${verdict.email ?? "operator"} attached to "${session}"`);

	// Either side going away takes the other with it, so a closed tab never
	// leaves a half-open connection to ttyd — the tmux session itself lives on.
	const close = () => {
		client.destroy();
		shell.destroy();
	};
	shell.on("error", (err) => {
		console.error("console gate: ttyd connection failed:", err.message);
		// Before the splice the client is still waiting for an HTTP answer; after
		// it, the connection is a WebSocket and the only honest reply is to close.
		if (spliced) close();
		else refuse(client, 502, "the console's shell is not running");
	});
	shell.on("close", close);
	client.on("error", close);
	client.on("close", close);
}

/** Answer a socket with a plain HTTP error and hang up. */
function refuse(client: Socket, status: number, reason: string): void {
	const body = `${reason}\n`;
	client.end(
		`HTTP/1.1 ${status} ${STATUS_CODES[status] ?? "Error"}\r\n` +
			"Content-Type: text/plain\r\n" +
			`Content-Length: ${Buffer.byteLength(body)}\r\n` +
			"Connection: close\r\n\r\n" +
			body,
	);
}

/** An environment variable the gate cannot start without. */
function required(name: string): string {
	const value = process.env[name];
	if (!value) {
		console.error(`console gate: ${name} is not set`);
		process.exit(1);
	}
	return value;
}

server.listen(port, () => console.log(`console gate: listening on :${port}`));
