/**
 * Reading the wire: a parser per agent frame that trusts none of it.
 *
 * The shapes themselves live in `frames.ts` and are re-exported here, so nothing
 * that imported this module has to know it was split. The SSE framing underneath
 * is in `lib/sse.ts`, shared with the Helm job stream.
 */

export type {
	AgentEvent,
	NavigateEvent,
	TextEvent,
	ThinkingEvent,
	ToolCallEvent,
	ToolResultEvent,
	TurnEndEvent,
	CompactionStartEvent,
	CompactionEndEvent,
	SignalEvent,
	DoneEvent,
	ErrorEvent,
	GuardrailEvent,
} from "./frames";

import type { AgentEvent, NavigateEvent } from "./frames";

const str = (v: unknown): v is string => typeof v === "string";

/** A finite number, or undefined. NaN and Infinity would render as themselves. */
const num = (v: unknown): number | undefined =>
	typeof v === "number" && Number.isFinite(v) ? v : undefined;

/**
 * Parse an agent frame, checking the fields the panel actually uses.
 *
 * Returns null for anything it cannot use rather than throwing. The type check
 * alone is not enough: the agent's definition is editable and its emit list is
 * open, so a frame can arrive well-formed as JSON and wrong in its fields — and
 * each of those has a consequence. A null `text` concatenates the word "null" into
 * the answer; a `tool_call` with no id opens a chip no result can ever close; a
 * non-string `error` reaches React as an object child and takes the panel down.
 */
export function parseAgentEvent(data: string): AgentEvent | null {
	let parsed: unknown;
	try {
		parsed = JSON.parse(data);
	} catch {
		return null;
	}
	if (!parsed || typeof parsed !== "object") return null;
	const frame = parsed as Record<string, unknown>;
	const iteration = num(frame.iteration);

	switch (frame.type) {
		case "text":
		case "thinking":
			if (!str(frame.text)) return null;
			return {
				type: frame.type,
				iteration,
				text: frame.text,
				index: num(frame.index),
			};

		case "tool_call":
			if (!str(frame.tool) || !str(frame.toolCallId)) return null;
			return {
				type: "tool_call",
				iteration,
				tool: frame.tool,
				toolCallId: frame.toolCallId,
				input: frame.input,
			};

		case "tool_result":
			if (!str(frame.tool) || !str(frame.toolCallId)) return null;
			return {
				type: "tool_result",
				iteration,
				tool: frame.tool,
				toolCallId: frame.toolCallId,
				output: frame.output,
				isError: Boolean(frame.isError),
			};

		// Both numbers or neither: half a gauge is worse than none, because the
		// budget is per block and nothing else on the wire says whether 12,000 is
		// comfortable or one turn from being compacted.
		case "turn_end": {
			const used = num(frame.contextTokens);
			const max = num(frame.contextMaxTokens);
			// A negative count is refused as well as a missing one. It cannot arrive
			// from a healthy runtime, but the gauge divides by the maximum and hands
			// the result to a progressbar's aria-valuenow — so a negative renders as
			// a negative percentage announced to a screen reader, which is a worse
			// outcome than showing no gauge at all.
			if (used === undefined || used < 0 || max === undefined || max <= 0) return null;
			return { type: "turn_end", iteration, contextTokens: used, contextMaxTokens: max };
		}

		case "compaction_start":
			return {
				type: "compaction_start",
				iteration,
				strategy: str(frame.strategy) ? frame.strategy : undefined,
			};

		case "compaction_end":
			return { type: "compaction_end", iteration, dropped: num(frame.dropped) };

		case "signal":
			if (!str(frame.signal)) return null;
			return {
				type: "signal",
				iteration,
				signal: frame.signal,
				text: str(frame.text) ? frame.text : undefined,
			};

		// The only one whose text is optional: it repeats what streamed, and the
		// reducer takes it only when nothing did.
		case "done":
			return { type: "done", iteration, text: str(frame.text) ? frame.text : undefined };

		case "error":
			if (!str(frame.error)) return null;
			return { type: "error", iteration, error: frame.error };

		case "guardrail":
			return {
				type: "guardrail",
				iteration,
				reason: str(frame.reason) ? frame.reason : undefined,
			};

		default:
			// A kind this build has never heard of is a configuration somebody chose,
			// not a failure. Skipping it beats throwing mid-stream.
			return null;
	}
}

/**
 * Parse a navigate frame, keeping only a path this app can actually route to.
 *
 * **This is the check that matters, and it belongs here rather than in the agent.**
 * The agent is a definition somebody can edit and redeploy, so a guard in its own
 * YAML is advice; this runs on every frame whatever that definition says.
 * A path must be site-relative — one leading slash, and not the `//host` form that
 * a browser reads as protocol-relative and follows off-site.
 */
export function parseNavigateEvent(data: string): NavigateEvent | null {
	let parsed: unknown;
	try {
		parsed = JSON.parse(data);
	} catch {
		return null;
	}
	if (!parsed || typeof parsed !== "object") return null;
	const { path, reason } = parsed as { path?: unknown; reason?: unknown };
	if (typeof path !== "string") return null;

	// Strip the characters a URL parser discards *before* testing anything, and
	// then use the stripped value. Checking the raw string would be checking a
	// different string than the browser acts on: "/\t/evil.example" starts with a
	// single slash and holds no backslash, so it passes every test below — and the
	// browser then removes the tab, leaving "//evil.example", which is
	// protocol-relative and off-site.
	const clean = path.replace(/[\t\n\r]/g, "");

	if (!clean.startsWith("/") || clean.startsWith("//")) return null;
	// A backslash is normalised to a forward slash by some browsers, so
	// "/\evil.example" can also leave the site. Nothing legitimate here has one.
	if (clean.includes("\\")) return null;
	return { path: clean, reason: typeof reason === "string" ? reason : undefined };
}

/**
 * Pull readable text out of the route's closing `answer` frame, whose data is the
 * flow's whole result body.
 *
 * Normally this repeats what already streamed and is discarded. It earns its keep
 * on the paths where nothing streamed: the agent's guardrail answers from a
 * `set-payload`, not from the model, so that reply exists *only* here.
 */
export function parseFinalAnswer(data: string): string | null {
	let parsed: unknown;
	try {
		parsed = JSON.parse(data);
	} catch {
		// Not JSON at all, so it is already the text.
		return data.trim() || null;
	}
	if (typeof parsed === "string") return parsed.trim() || null;
	if (parsed && typeof parsed === "object") {
		// `answer` is what the agent's own guardrail uses; `text` and `message` are
		// what an edited one is most likely to reach for.
		for (const key of ["answer", "text", "message"]) {
			const value = (parsed as Record<string, unknown>)[key];
			if (typeof value === "string" && value.trim()) return value.trim();
		}
	}
	return null;
}
