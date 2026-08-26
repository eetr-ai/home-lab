import { describe, expect, it } from "vitest";
import type { AgentEvent } from "./frames";
import {
	acknowledge,
	answerOf,
	newTurn,
	reduce,
	settle,
	takeIn,
	type Segment,
	type Turn,
} from "./turns";

/** Fold a whole run into one turn, the way the reader loop does. */
function run(...events: AgentEvent[]): Turn {
	return events.reduce(reduce, { ...newTurn("a", "agent"), streaming: true });
}

const kinds = (turn: Turn) => turn.segments.map((s) => s.kind);

describe("reduce", () => {
	it("accumulates text into one segment", () => {
		const turn = run(
			{ type: "text", text: "he", iteration: 1 },
			{ type: "text", text: "llo", iteration: 1 },
		);
		expect(answerOf(turn)).toBe("hello");
		expect(kinds(turn)).toEqual(["text"]);
	});

	// The iteration is the agent's own turn counter. Two rounds of the same kind of
	// work are two things it did, and merging them hides the model call between.
	it("starts a new segment when the iteration moves on", () => {
		const turn = run(
			{ type: "thinking", text: "first", iteration: 1 },
			{ type: "thinking", text: "second", iteration: 2 },
		);
		expect(kinds(turn)).toEqual(["thinking", "thinking"]);
	});

	// The whole reason a turn is an ordered list rather than a set of buckets: a run
	// that thinks, calls a tool, thinks again and answers did those in that order.
	it("keeps segments in the order they happened", () => {
		const turn = run(
			{ type: "thinking", text: "hmm", iteration: 1 },
			{ type: "tool_call", tool: "admin_read", toolCallId: "1", iteration: 1 },
			{ type: "thinking", text: "aha", iteration: 2 },
			{ type: "text", text: "here", iteration: 2 },
		);
		expect(kinds(turn)).toEqual(["thinking", "tools", "thinking", "text"]);
	});

	it("groups one round of tool calls together and separates the next", () => {
		const turn = run(
			{ type: "tool_call", tool: "a", toolCallId: "1", iteration: 1 },
			{ type: "tool_call", tool: "b", toolCallId: "2", iteration: 1 },
			{ type: "tool_call", tool: "c", toolCallId: "3", iteration: 2 },
		);
		const tools = turn.segments.filter(
			(s): s is Extract<Segment, { kind: "tools" }> => s.kind === "tools",
		);
		expect(tools.map((s) => s.runs.length)).toEqual([2, 1]);
	});

	// A result arrives after the branch that ran it returned, by which time the
	// agent may have opened another round — so it cannot be closed on the last
	// segment alone.
	it("closes a tool in an earlier round", () => {
		const turn = run(
			{ type: "tool_call", tool: "a", toolCallId: "1", iteration: 1 },
			{ type: "tool_call", tool: "b", toolCallId: "2", iteration: 2 },
			{ type: "tool_result", tool: "a", toolCallId: "1", output: { ok: true }, iteration: 2 },
		);
		const first = turn.segments[0] as Extract<Segment, { kind: "tools" }>;
		expect(first.runs[0]).toMatchObject({ done: true, failed: false, output: { ok: true } });
	});

	it("marks a failed tool", () => {
		const turn = run(
			{ type: "tool_call", tool: "run_command", toolCallId: "1", iteration: 1 },
			{ type: "tool_result", tool: "run_command", toolCallId: "1", isError: true, iteration: 1 },
		);
		const tools = turn.segments[0] as Extract<Segment, { kind: "tools" }>;
		expect(tools.runs[0]).toMatchObject({ done: true, failed: true });
	});

	// A result for a call this panel never saw open must not invent a segment.
	it("ignores a result for an unknown call", () => {
		const turn = run({ type: "tool_result", tool: "a", toolCallId: "ghost", iteration: 1 });
		expect(turn.segments).toEqual([]);
	});

	it("brackets a compaction and records what it dropped", () => {
		const turn = run(
			{ type: "compaction_start", strategy: "summarize", iteration: 3 },
			{ type: "compaction_end", dropped: 12, iteration: 3 },
		);
		expect(turn.segments[0]).toMatchObject({ kind: "compaction", done: true, dropped: 12 });
	});

	it("records the context gauge from a finished model turn", () => {
		const turn = run({
			type: "turn_end",
			contextTokens: 1_000,
			contextMaxTokens: 4_000,
			iteration: 1,
		});
		expect(turn.context).toEqual({ used: 1_000, max: 4_000 });
	});

	// `done` repeats what streamed. Taking it as well would print the answer twice.
	it("ignores the final answer when text already streamed", () => {
		const turn = run(
			{ type: "text", text: "streamed", iteration: 1 },
			{ type: "done", text: "streamed", iteration: 1 },
		);
		expect(answerOf(turn)).toBe("streamed");
	});

	it("takes the final answer when nothing streamed", () => {
		expect(answerOf(run({ type: "done", text: "only here", iteration: 1 }))).toBe("only here");
	});

	it("notes an error rather than losing it", () => {
		expect(run({ type: "error", error: "the model refused", iteration: 1 }).note).toBe(
			"the model refused",
		);
	});

	// The reason is diagnostic and written for a log; the reply itself comes from
	// the guardrail's own payload and arrives on the closing frame.
	it.each([
		["model refused", "It declined this one."],
		["exceeded max iterations", "It ran out of steps"],
		["something nobody has seen", "It stopped short of an answer."],
	])("turns the guardrail reason %s into something a reader can act on", (reason, expected) => {
		expect(run({ type: "guardrail", reason, iteration: 9 }).note).toContain(expected);
	});
});

describe("takeIn", () => {
	const pending = (id: string, text: string): Turn => ({
		...newTurn(id, "user", text),
		delivery: "pending",
	});

	// A steered message is held at the bottom until the run reads it. The moment it
	// does, it belongs in the middle of the conversation: after what the run had
	// already done, and before everything it does because of the message.
	it("moves a waiting message to where the run read it", () => {
		const turns: Turn[] = [
			newTurn("q", "user", "first question"),
			{ ...newTurn("a", "agent"), streaming: true, segments: [{ kind: "text", iter: 1, text: "…" }] },
			pending("s", "actually, check the nodes"),
		];

		const after = takeIn(turns, "a", "a2", "actually, check the nodes");

		expect(after.map((t) => t.id)).toEqual(["q", "a", "s", "a2"]);
		expect(after[2].delivery).toBe("taken");
		expect(after[3]).toMatchObject({ role: "agent", streaming: true });
	});

	// A message this window never sent — a second tab — really did shape what
	// follows, so a reply that changes direction has something to change about.
	it("writes in a message this window never sent", () => {
		const turns: Turn[] = [{ ...newTurn("a", "agent"), streaming: true }];
		const after = takeIn(turns, "a", "a2", "from another tab");
		expect(after.map((t) => t.role)).toEqual(["user", "agent"]);
		expect(answerOf(after[0])).toBe("from another tab");
	});

	// Two messages read in one iteration would otherwise leave an empty turn
	// between them.
	it("drops the closing turn when it has nothing in it", () => {
		const turns: Turn[] = [{ ...newTurn("a", "agent"), streaming: true }, pending("s", "hello")];
		expect(takeIn(turns, "a", "a2", "hello").map((t) => t.id)).toEqual(["s", "a2"]);
	});

	// The gauge is a property of the conversation, not of the turn: blanking it
	// would read as the context having been lost along with the turn.
	it("carries the context gauge across", () => {
		const turns: Turn[] = [
			{ ...newTurn("a", "agent"), streaming: true, context: { used: 10, max: 100 }, segments: [{ kind: "text", iter: 1, text: "x" }] },
		];
		expect(takeIn(turns, "a", "a2", "hi").at(-1)?.context).toEqual({ used: 10, max: 100 });
	});
});

describe("acknowledge", () => {
	const turns: Turn[] = [{ ...newTurn("s", "user", "did you see this"), delivery: "pending" }];

	it("marks a message the run never answered", () => {
		expect(acknowledge(turns, "unanswered", "did you see this")?.[0].delivery).toBe("missed");
	});

	// The one-element list above would pass even if every entry were rewritten, so
	// this pins the two properties `Array.prototype.with` used to provide here and
	// a hand-written copy has to keep: exactly one index changes, and the array
	// that went in is not touched.
	it("marks only that message, and leaves the input alone", () => {
		const many: Turn[] = [
			{ ...newTurn("a", "user", "first"), delivery: "pending" },
			{ ...newTurn("b", "user", "second"), delivery: "pending" },
			{ ...newTurn("c", "user", "third"), delivery: "pending" },
		];
		const after = acknowledge(many, "unanswered", "second");
		expect(after?.map((t) => t.delivery)).toEqual(["pending", "missed", "pending"]);
		expect(many.map((t) => t.delivery)).toEqual(["pending", "pending", "pending"]);
	});

	// Null is how the caller learns this window did not send it, and falls back to
	// saying so in the transcript instead.
	it("returns null for a message it cannot find", () => {
		expect(acknowledge(turns, "unanswered", "something else")).toBeNull();
		expect(acknowledge(turns, "context", "did you see this")).toBeNull();
	});
});

describe("settle", () => {
	// An acknowledgement only ever arrives on the stream the run owns, so anything
	// still pending when it closes was never taken. Left alone it is a spinner that
	// never resolves.
	it("gives up on anything still waiting", () => {
		const turns: Turn[] = [
			{ ...newTurn("s", "user", "hi"), delivery: "pending" },
			{ ...newTurn("t", "user", "ok"), delivery: "taken" },
		];
		expect(settle(turns).map((t) => t.delivery)).toEqual(["missed", "taken"]);
	});
});
