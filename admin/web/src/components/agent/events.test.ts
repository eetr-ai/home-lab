import { describe, expect, it } from "vitest";
import { parseAgentEvent, parseFinalAnswer, parseNavigateEvent, parseSSE } from "./events";

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

describe("parseAgentEvent", () => {
	it("reads a text frame", () => {
		expect(parseAgentEvent('{"type":"text","text":"hi","iteration":2}')).toEqual({
			type: "text",
			text: "hi",
			iteration: 2,
			index: undefined,
		});
	});

	// Each of these has a consequence: a null text concatenates "null" into the
	// answer, a call with no id opens a chip no result can close, and a non-string
	// error reaches React as an object child and takes the panel down.
	it.each([
		["text with no string", '{"type":"text","text":null}'],
		["a tool call with no id", '{"type":"tool_call","tool":"admin_read"}'],
		["a tool result with no tool", '{"type":"tool_result","toolCallId":"1"}'],
		["an error that is not a string", '{"type":"error","error":{"message":"x"}}'],
		["a type this build has never heard of", '{"type":"telepathy"}'],
		["something that is not JSON", "not json at all"],
		["JSON that is not an object", "42"],
	])("refuses %s", (_name, data) => {
		expect(parseAgentEvent(data)).toBeNull();
	});

	// Half a gauge is worse than none: 12,000 says nothing without the budget.
	it.each([
		['{"type":"turn_end","contextTokens":10}', "no maximum"],
		['{"type":"turn_end","contextMaxTokens":100}', "no usage"],
		['{"type":"turn_end","contextTokens":10,"contextMaxTokens":0}', "a zero maximum"],
	])("refuses a turn_end with %s", (data) => {
		expect(parseAgentEvent(data)).toBeNull();
	});

	// NaN and Infinity are numbers, and would render as themselves.
	it("drops a non-finite iteration rather than carrying it", () => {
		const parsed = parseAgentEvent('{"type":"text","text":"hi","iteration":1e999}');
		expect(parsed).toMatchObject({ type: "text", iteration: undefined });
	});
});

describe("parseNavigateEvent", () => {
	it("takes a site-relative path", () => {
		expect(parseNavigateEvent('{"path":"/kubernetes/nodes","reason":"here"}')).toEqual({
			path: "/kubernetes/nodes",
			reason: "here",
		});
	});

	// This is the boundary, not the agent's definition — so every way out of the
	// site has to be refused here, whatever the definition was changed to say.
	it.each([
		["a protocol-relative path", '{"path":"//evil.example"}'],
		["an absolute URL", '{"path":"https://evil.example"}'],
		["a backslash some browsers normalise", '{"path":"/\\\\evil.example"}'],
		["a relative path", '{"path":"kubernetes"}'],
		["a path that is not a string", '{"path":42}'],
	])("refuses %s", (_name, data) => {
		expect(parseNavigateEvent(data)).toBeNull();
	});

	// The characters a URL parser discards have to go before anything is tested:
	// this one starts with a single slash and holds no backslash, and the browser
	// then removes the tab and follows it off-site.
	it("refuses a path that becomes protocol-relative once stripped", () => {
		const frame = '{"path":"/\\t/evil.example"}';
		// Asserted, because the whole test would pass for the wrong reason if this
		// were merely invalid JSON: the point is a path that survives parsing, looks
		// site-relative, and stops being so once the browser drops the tab.
		expect(JSON.parse(frame).path).toBe("/\t/evil.example");
		expect(parseNavigateEvent(frame)).toBeNull();
	});
});

describe("parseFinalAnswer", () => {
	// The guardrail answers from a set-payload rather than from the model, so this
	// closing frame is the only place that reply exists.
	it("reads the guardrail's answer field", () => {
		expect(parseFinalAnswer('{"answer":"  I stopped short.  "}')).toBe("I stopped short.");
	});

	it.each([["text"], ["message"]])("reads the %s field of an edited definition", (key) => {
		expect(parseFinalAnswer(JSON.stringify({ [key]: "hello" }))).toBe("hello");
	});

	it("treats a non-JSON body as the text itself", () => {
		expect(parseFinalAnswer("plain words")).toBe("plain words");
	});

	it("returns null when there is nothing to say", () => {
		expect(parseFinalAnswer('{"navigated":true}')).toBeNull();
		expect(parseFinalAnswer("   ")).toBeNull();
	});
});
