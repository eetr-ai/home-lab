"use client";

import { parseSSE } from "@/lib/sse";
import { parseJobEvent, type JobEvent } from "./job-events";

/**
 * One job stream, read to its end.
 *
 * Split from the hook because it is neither state nor request: given a body and
 * somewhere to put the events, it reads until the stream stops. The agent
 * drawer's readRun does the same job for the same reason.
 *
 * Returns whether the stream ended with a terminal event. That distinction is the
 * caller's whole reconnect rule — an EOF after `done` is the operation finishing,
 * and an EOF without one is a dropped connection, which happens routinely when
 * the panel's own pods are replaced by the upgrade being watched.
 */
export async function readJobStream(
	body: ReadableStream<Uint8Array>,
	onEvent: (event: JobEvent) => void,
): Promise<{ ended: boolean }> {
	let ended = false;

	for await (const frame of parseSSE(body)) {
		const event = parseJobEvent(frame.event, frame.data);
		if (!event) continue;
		onEvent(event);
		if (event.type === "done") ended = true;
	}

	return { ended };
}
