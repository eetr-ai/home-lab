import { describe, expect, it } from "vitest";
import { parseSSE } from "./sse";

/** A stream that hands over exactly these chunks, so a split can be placed. */
function streamOf(...chunks: string[]): ReadableStream<Uint8Array> {
	const encoder = new TextEncoder();
	return new ReadableStream({
		start(controller) {
			for (const chunk of chunks) controller.enqueue(encoder.encode(chunk));
			controller.close();
		},
	});
}

async function framesOf(...chunks: string[]) {
	const out = [];
	for await (const frame of parseSSE(streamOf(...chunks))) out.push(frame);
	return out;
}

describe("parseSSE", () => {
	it("reads a frame's event name and data", async () => {
		expect(await framesOf('event: agent\ndata: {"type":"text"}\n\n')).toEqual([
			{ event: "agent", data: '{"type":"text"}' },
		]);
	});

	// A frame is split wherever TCP decides, and a token stream splits constantly.
	it("holds a frame that arrives in pieces", async () => {
		expect(await framesOf("event: ag", "ent\ndata: hel", "lo\n\n")).toEqual([
			{ event: "agent", data: "hello" },
		]);
	});

	it("reads several frames out of one chunk", async () => {
		const frames = await framesOf("event: a\ndata: 1\n\nevent: b\ndata: 2\n\n");
		expect(frames.map((f) => f.event)).toEqual(["a", "b"]);
	});

	// The format permits \r\n and a proxy may rewrite line endings.
	it("tolerates carriage returns", async () => {
		expect(await framesOf("event: a\r\ndata: 1\r\n\r\n")).toEqual([{ event: "a", data: "1" }]);
	});

	it("joins repeated data lines with newlines", async () => {
		expect(await framesOf("data: one\ndata: two\n\n")).toEqual([
			{ event: "message", data: "one\ntwo" },
		]);
	});

	// Heartbeats arrive as comments, and one must not read as an empty frame.
	it("skips comment lines", async () => {
		expect(await framesOf(": keep-alive\n\ndata: real\n\n")).toEqual([
			{ event: "message", data: "real" },
		]);
	});

	// The agent's run can end without a trailing blank line — and the last frame is
	// the answer, so losing it would be losing the reply.
	it("yields a final frame with no trailing blank line", async () => {
		expect(await framesOf("event: answer\ndata: done")).toEqual([
			{ event: "answer", data: "done" },
		]);
	});

	// A multi-byte character split across a chunk boundary lives in the decoder
	// until it is flushed.
	it("reassembles a character split across chunks", async () => {
		const bytes = new TextEncoder().encode("data: é\n\n");
		const frames = [];
		const stream = new ReadableStream<Uint8Array>({
			start(controller) {
				controller.enqueue(bytes.slice(0, 7));
				controller.enqueue(bytes.slice(7));
				controller.close();
			},
		});
		for await (const frame of parseSSE(stream)) frames.push(frame);
		expect(frames).toEqual([{ event: "message", data: "é" }]);
	});
});
