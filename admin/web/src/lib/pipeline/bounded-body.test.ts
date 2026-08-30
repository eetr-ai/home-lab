import { describe, expect, it } from "vitest";
import { MAX_BODY_BYTES, readBounded } from "./bounded-body";

/** A request whose body arrives in chunks, optionally lying about its length. */
function streamed(chunks: string[], contentLength?: string): Request {
	const encoder = new TextEncoder();
	const body = new ReadableStream<Uint8Array>({
		start(controller) {
			for (const chunk of chunks) controller.enqueue(encoder.encode(chunk));
			controller.close();
		},
	});
	const headers = new Headers();
	if (contentLength !== undefined) headers.set("content-length", contentLength);
	// `duplex` is required whenever the body is a stream and is absent from the
	// DOM's RequestInit, so the cast is the only way to construct one.
	return new Request("https://panel.invalid/api/v1/charts/x", {
		method: "PATCH",
		headers,
		body,
		duplex: "half",
	} as RequestInit & { duplex: "half" });
}

describe("readBounded", () => {
	it("reads a body that fits", async () => {
		const request = new Request("https://panel.invalid/api/v1/charts/x", {
			method: "PATCH",
			body: '{"chartVersion":"6.9.4"}',
		});
		await expect(readBounded(request)).resolves.toEqual({
			text: '{"chartVersion":"6.9.4"}',
		});
	});

	it("reassembles a body that arrived in pieces", async () => {
		const result = await readBounded(streamed(['{"chartVer', 'sion":"6.9.4"}']));
		expect(result).toEqual({ text: '{"chartVersion":"6.9.4"}' });
	});

	it("refuses a body that declares itself too large", async () => {
		const request = new Request("https://panel.invalid/api/v1/charts/x", {
			method: "PATCH",
			headers: { "content-length": String(MAX_BODY_BYTES + 1) },
			body: "{}",
		});
		await expect(readBounded(request)).resolves.toEqual({ tooLarge: true });
	});

	// The one the header check cannot catch: a chunked request declares no length,
	// or declares a length it has no intention of honouring.
	it("refuses a body that overruns while streaming, declared or not", async () => {
		await expect(readBounded(streamed(["ab", "cd", "ef"], undefined), 4)).resolves.toEqual({
			tooLarge: true,
		});
		await expect(readBounded(streamed(["ab", "cd", "ef"], "2"), 4)).resolves.toEqual({
			tooLarge: true,
		});
	});

	it("accepts a body exactly at the limit", async () => {
		const result = await readBounded(streamed(["abcd"]), 4);
		expect(result).toEqual({ text: "abcd" });
	});

	it("reads an absent body as empty rather than failing", async () => {
		const request = new Request("https://panel.invalid/api/v1/charts/x", { method: "PATCH" });
		await expect(readBounded(request)).resolves.toEqual({ text: "" });
	});
});
