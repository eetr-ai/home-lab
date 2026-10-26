"use client";

import { useEffect, useRef, useState } from "react";
import { parseSSE } from "@/lib/sse";
import { parseScheduleEvent, runFinished, type ScheduleState } from "@/lib/backups/schedule-events";

/** How long to wait before reconnecting, and the ceiling it backs off to. */
const FIRST_RETRY_MS = 1_000;
const MAX_RETRY_MS = 15_000;

/**
 * Follows a schedule for as long as its page is open.
 *
 * Starts from what the server rendered and replaces it with every state the
 * stream sends, so the runs and the last success move without a reload. When a
 * run finishes it calls onRunFinished — the page refreshes what is kept in Drive
 * then, because that is the moment it changes and the stream does not carry it.
 *
 * The stream never ends on its own, so any end is a dropped connection and is
 * followed by a reconnect, backing off while it keeps failing. Each connection
 * starts with the whole state, so nothing is lost across one.
 */
export function useScheduleStream(
	initial: ScheduleState,
	initialAt: Date,
	onRunFinished: () => void,
): { state: ScheduleState; at: Date } {
	// `at` is when the state was read, so ages on the page count from the reading
	// rather than from a clock read during render, which would differ between the
	// server and the browser.
	const [live, setLive] = useState({ state: initial, at: initialAt });
	const runs = useRef(initial.runs);
	// Read through a ref so a new callback each render does not reconnect.
	const finished = useRef(onRunFinished);
	useEffect(() => {
		finished.current = onRunFinished;
	}, [onRunFinished]);

	const name = initial.schedule.name;
	useEffect(() => {
		const controller = new AbortController();
		let retry: ReturnType<typeof setTimeout> | undefined;
		let attempt = 0;

		async function follow() {
			try {
				const response = await fetch(`/api/backups/schedule-events?name=${encodeURIComponent(name)}`, {
					signal: controller.signal,
					cache: "no-store",
				});
				if (!response.ok || !response.body) throw new Error(`the API answered ${response.status}`);

				for await (const frame of parseSSE(response.body)) {
					const event = parseScheduleEvent(frame.event, frame.data);
					if (event?.type !== "state") continue;
					attempt = 0;
					if (runFinished(runs.current, event.state.runs)) finished.current();
					runs.current = event.state.runs;
					setLive({ state: event.state, at: new Date() });
				}
			} catch {
				// An abort is the page going away, which is how this normally ends.
				if (controller.signal.aborted) return;
			}
			if (controller.signal.aborted) return;
			retry = setTimeout(follow, Math.min(FIRST_RETRY_MS * 2 ** attempt++, MAX_RETRY_MS));
		}

		void follow();
		return () => {
			// Aborting is what ends the API's loop reading the cluster for this tab.
			controller.abort();
			if (retry !== undefined) clearTimeout(retry);
		};
	}, [name]);

	return live;
}
