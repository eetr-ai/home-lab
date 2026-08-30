/**
 * Server-sent events, as a byte stream turned into frames.
 *
 * Transport only. Nothing here knows what a frame *means* — the agent's parser
 * and the Helm job's parser each read `data` their own way, and both start from
 * this. It lives under `lib/` rather than beside either of them because a second
 * copy of a framing parser is a second set of bugs about split chunks and
 * carriage returns, and only one of the copies would get the fix.
 */

/** One parsed server-sent event: its `event:` name and its `data:` payload. */
export interface SSEFrame {
	event: string;
	data: string;
}

/** The default event name a frame carries when it declares none. */
const DEFAULT_EVENT = "message";

/**
 * Turn a byte stream of server-sent events into frames.
 *
 * Written as a generator over the raw reader rather than using EventSource for two
 * reasons that both still hold: the agent's chat request is a POST with a body,
 * which EventSource cannot make, and an `AbortSignal` cannot be attached to one —
 * which is what tears a watch down at the API server when a tab closes.
 *
 * It holds the partial tail between chunks — a frame is split wherever TCP decides,
 * and a token stream splits often — and joins repeated `data:` lines with newlines,
 * as the format requires.
 */
export async function* parseSSE(
	stream: ReadableStream<Uint8Array>,
): AsyncGenerator<SSEFrame> {
	const reader = stream.getReader();
	const decoder = new TextDecoder();
	let buffer = "";

	try {
		for (;;) {
			const { done, value } = await reader.read();
			if (done) break;
			buffer += decoder.decode(value, { stream: true });

			// Frames are separated by a blank line. \r\n is tolerated because the format
			// permits it and a proxy may rewrite line endings.
			let split = buffer.search(/\r?\n\r?\n/);
			while (split !== -1) {
				const raw = buffer.slice(0, split);
				buffer = buffer.slice(split + /\r?\n\r?\n/.exec(buffer.slice(split))![0].length);
				const frame = parseFrame(raw);
				if (frame) yield frame;
				split = buffer.search(/\r?\n\r?\n/);
			}
		}
		// Flush any bytes the decoder is still holding — a multi-byte character split
		// across the last chunk boundary lives there until asked for.
		buffer += decoder.decode();
		// A stream that ends without a trailing blank line still had a frame in it.
		const frame = parseFrame(buffer);
		if (frame) yield frame;
	} finally {
		reader.releaseLock();
	}
}

/** Read one frame's field lines into an event name and its data. */
function parseFrame(raw: string): SSEFrame | null {
	let event = "";
	const data: string[] = [];

	for (const line of raw.split(/\r?\n/)) {
		// A line beginning with a colon is a comment; heartbeats arrive as those.
		if (line === "" || line.startsWith(":")) continue;
		const colon = line.indexOf(":");
		const field = colon === -1 ? line : line.slice(0, colon);
		// Exactly one optional leading space after the colon is part of the format.
		let value = colon === -1 ? "" : line.slice(colon + 1);
		if (value.startsWith(" ")) value = value.slice(1);

		if (field === "event") event = value;
		else if (field === "data") data.push(value);
	}

	if (data.length === 0) return null;
	return { event: event || DEFAULT_EVENT, data: data.join("\n") };
}
