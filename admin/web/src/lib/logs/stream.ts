/**
 * The bookkeeping a live log tail needs, kept out of the component so it can be
 * tested — vitest here runs in node and covers `src/lib` only, deliberately.
 */

/**
 * How many lines the viewer retains.
 *
 * A pod that has been up for weeks and is chatty would otherwise grow the DOM
 * without limit, and a browser tab that slows to a stop is a worse outcome than
 * losing the oldest lines of a tail nobody scrolled back to.
 */
export const MAX_LINES = 5000;

/**
 * Split a decoded chunk into complete lines and whatever is left over.
 *
 * A chunk boundary lands mid-line often enough to matter — it is a network read,
 * not a line read — so the trailing fragment is carried forward rather than
 * rendered as a line of its own. Rendering it would show every long line broken
 * in two for one frame.
 */
export function splitLines(pending: string, chunk: string): { lines: string[]; pending: string } {
	const parts = (pending + chunk).split("\n");
	// The last element is either a partial line or "" when the chunk ended cleanly.
	// Both are the right thing to carry: "" concatenates harmlessly next time.
	const remainder = parts.pop() ?? "";
	return { lines: parts, pending: remainder };
}

/** Append to a bounded buffer, dropping the oldest lines past the limit. */
export function appendBounded(existing: string[], lines: string[], max = MAX_LINES): string[] {
	if (lines.length === 0) return existing;
	const next = existing.concat(lines);
	return next.length > max ? next.slice(next.length - max) : next;
}

/**
 * How close to the bottom counts as "following the tail".
 *
 * Not zero: a smooth scroll, a fractional device pixel ratio, and a line that
 * arrives mid-scroll all leave a pixel or two of slack, and requiring an exact
 * bottom would unpin the view constantly for no reason a person could see.
 */
const PINNED_WITHIN_PX = 24;

/**
 * Whether new lines should scroll the view.
 *
 * Pinned to the bottom means the reader is watching the tail and wants to keep
 * watching it. Scrolled up means they are reading something, and yanking them
 * back down every time a line arrives makes the panel unusable exactly when it
 * matters — while something is failing loudly.
 */
export function isPinnedToBottom(
	scrollHeight: number,
	scrollTop: number,
	clientHeight: number,
): boolean {
	return scrollHeight - scrollTop - clientHeight < PINNED_WITHIN_PX;
}

/** Build the log route's query string from what the viewer is showing. */
export function logStreamUrl(options: {
	namespace: string;
	pod: string;
	container?: string;
	follow?: boolean;
	tail?: number;
	previous?: boolean;
}): string {
	const params = new URLSearchParams({ namespace: options.namespace, pod: options.pod });
	if (options.container) params.set("container", options.container);
	if (options.follow) params.set("follow", "true");
	if (options.previous) params.set("previous", "true");
	if (options.tail !== undefined) params.set("tail", String(options.tail));
	return `/api/kubernetes/logs?${params.toString()}`;
}
