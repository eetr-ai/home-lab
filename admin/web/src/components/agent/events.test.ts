import { describe, expect, it } from "vitest";
import { parseAgentEvent, parseFinalAnswer, parseNavigateEvent } from "./events";

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
		// The gauge divides by the maximum and feeds aria-valuenow, so a negative
		// would be announced to a screen reader as a negative percentage.
		['{"type":"turn_end","contextTokens":-1,"contextMaxTokens":100}', "negative usage"],
		['{"type":"turn_end","contextTokens":10,"contextMaxTokens":-100}', "a negative maximum"],
	])("refuses a turn_end with %s", (data) => {
		expect(parseAgentEvent(data)).toBeNull();
	});

	it("accepts a fresh conversation reporting zero used", () => {
		expect(parseAgentEvent('{"type":"turn_end","contextTokens":0,"contextMaxTokens":100}')).toMatchObject(
			{ contextTokens: 0, contextMaxTokens: 100 },
		);
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

	// The scope of several pages lives in the query string — ?database=, ?namespace=
	// — so a navigation that loses it lands on the right page showing the wrong
	// thing, which is worse than not navigating. The catalogue the agent is given
	// names those parameters, and this is what says they survive the boundary.
	it("keeps a query string, which is where a page's scope lives", () => {
		expect(parseNavigateEvent('{"path":"/postgres/query?database=orders"}')).toEqual({
			path: "/postgres/query?database=orders",
			reason: undefined,
		});
	});

	it("keeps more than one parameter, and an encoded value", () => {
		const path = "/kubernetes/pods?namespace=kube-system&q=web%20server";
		expect(parseNavigateEvent(JSON.stringify({ path }))?.path).toBe(path);
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
