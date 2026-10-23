import { describe, expect, it } from "vitest";
import {
	decodeServerMessage,
	initMessage,
	inputMessage,
	resizeMessage,
	socketUrl,
} from "./ttyd";

const text = (bytes: Uint8Array) => new TextDecoder().decode(bytes);
const bytes = (s: string) => new TextEncoder().encode(s);

describe("client messages", () => {
	it("opens with the size as JSON, which is what starts the shell", () => {
		expect(JSON.parse(initMessage({ columns: 120, rows: 40 }))).toEqual({
			AuthToken: "",
			columns: 120,
			rows: 40,
		});
	});

	it("prefixes input with the input command", () => {
		expect(text(inputMessage("ls -la\r"))).toBe("0ls -la\r");
	});

	it("encodes input as UTF-8, not as UTF-16 code units", () => {
		const message = inputMessage("é");
		expect(Array.from(message)).toEqual([0x30, 0xc3, 0xa9]);
	});

	it("prefixes a resize with the resize command", () => {
		const message = text(resizeMessage({ columns: 80, rows: 24 }));
		expect(message[0]).toBe("1");
		expect(JSON.parse(message.slice(1))).toEqual({ columns: 80, rows: 24 });
	});
});

describe("decodeServerMessage", () => {
	it("keeps output as bytes after the command", () => {
		const decoded = decodeServerMessage(bytes("0hello"));
		expect(decoded.kind).toBe("output");
		if (decoded.kind === "output") expect(text(decoded.data)).toBe("hello");
	});

	it("does not split a character a message boundary already split", () => {
		const half = new Uint8Array([0x30, 0xc3]);
		const decoded = decodeServerMessage(half.buffer);
		expect(decoded).toEqual({ kind: "output", data: new Uint8Array([0xc3]) });
	});

	it("reads a window title", () => {
		expect(decodeServerMessage(bytes("1lab@console: ~"))).toEqual({
			kind: "title",
			title: "lab@console: ~",
		});
	});

	it("recognises preferences without acting on them", () => {
		expect(decodeServerMessage(bytes('2{"fontSize":14}'))).toEqual({ kind: "preferences" });
	});

	it("ignores an empty or unknown message", () => {
		expect(decodeServerMessage(new Uint8Array())).toEqual({ kind: "unknown" });
		expect(decodeServerMessage(bytes("9?"))).toEqual({ kind: "unknown" });
	});
});

describe("socketUrl", () => {
	const page = { protocol: "https:", host: "admin.example.invalid" };

	it("puts a path on the panel's own origin, as wss behind https", () => {
		expect(socketUrl("/console/socket", "main", page)).toBe(
			"wss://admin.example.invalid/console/socket?session=main",
		);
	});

	it("uses plain ws behind plain http", () => {
		expect(socketUrl("/console/socket", "a", { protocol: "http:", host: "localhost:3000" })).toBe(
			"ws://localhost:3000/console/socket?session=a",
		);
	});

	it("takes a full socket URL as it is, for a gate on another port", () => {
		expect(socketUrl("ws://localhost:3001/console/socket", "b", page)).toBe(
			"ws://localhost:3001/console/socket?session=b",
		);
	});

	it("escapes the session", () => {
		expect(socketUrl("/s", "a b", page)).toBe("wss://admin.example.invalid/s?session=a+b");
	});
});
