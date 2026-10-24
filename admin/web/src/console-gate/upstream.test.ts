import { describe, expect, it } from "vitest";
import { upstreamHead } from "./upstream";

describe("upstreamHead", () => {
	const browser = {
		host: "admin.example.invalid",
		upgrade: "websocket",
		connection: "Upgrade",
		"sec-websocket-key": "dGhlIHNhbXBsZSBub25jZQ==",
		"sec-websocket-version": "13",
		"sec-websocket-protocol": "tty",
		cookie: "__Secure-authjs.session-token=sealed",
		origin: "https://admin.example.invalid",
	};

	const head = upstreamHead(browser, "deploys", "127.0.0.1:7681");
	const lines = head.split("\r\n");

	it("asks ttyd's socket for the session as its argument", () => {
		expect(lines[0]).toBe("GET /ws?arg=deploys HTTP/1.1");
	});

	it("names ttyd as the host, not the panel", () => {
		expect(lines).toContain("Host: 127.0.0.1:7681");
		expect(head).not.toContain("admin.example.invalid");
	});

	it("keeps the handshake intact", () => {
		expect(lines).toContain("upgrade: websocket");
		expect(lines).toContain("connection: Upgrade");
		expect(lines).toContain("sec-websocket-key: dGhlIHNhbXBsZSBub25jZQ==");
		expect(lines).toContain("sec-websocket-version: 13");
		expect(lines).toContain("sec-websocket-protocol: tty");
	});

	it("leaves the operator's cookie behind", () => {
		expect(head.toLowerCase()).not.toContain("cookie");
		expect(head).not.toContain("sealed");
	});

	it("ends with the blank line that closes a head", () => {
		expect(head.endsWith("\r\n\r\n")).toBe(true);
		expect(head.slice(0, -4)).not.toContain("\r\n\r\n");
	});

	it("joins a header Node collected more than once", () => {
		const twice = upstreamHead(
			{ ...browser, "sec-websocket-extensions": ["a", "b"] },
			"main",
			"127.0.0.1:7681",
		);
		expect(twice).toContain("sec-websocket-extensions: a, b\r\n");
	});

	it("escapes the session, should a caller skip the name check", () => {
		expect(upstreamHead(browser, "a b&c", "h:1").split("\r\n")[0]).toBe(
			"GET /ws?arg=a%20b%26c HTTP/1.1",
		);
	});
});
