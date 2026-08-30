"use client";

import { useEffect, useReducer, useRef, useState } from "react";

import { foldJobEvent, initialJobState, type JobState } from "./job-state";
import { readJobStream } from "./read-job-stream";
import type { HelmJob } from "@/lib/api/types";

/** How long to wait before reconnecting, and the ceiling it backs off to. */
const FIRST_RETRY_MS = 1_000;
const MAX_RETRY_MS = 15_000;

/**
 * Follows a Helm job until it ends.
 *
 * The React is all here; the parsing and the folding are in modules beside this
 * one that have none, which is where the tests are.
 *
 * Reconnecting is the normal path rather than the exceptional one. Rolling out
 * the panel's own chart replaces both the pod serving this stream and the pod
 * proxying it, so the connection drops every time — and the operation carries on
 * regardless, in a Job that nothing is replacing. When the new pods are ready this
 * reconnects, gets a fresh snapshot, and picks the job up wherever it now is.
 *
 * There is no resume token, and there does not need to be: a snapshot is the whole
 * present truth, so re-deriving costs one frame and needs no state on either side.
 */
export function useJobStream(name: string | null, initial: HelmJob | null = null): JobState {
	const [state, apply] = useReducer(foldJobEvent, initial, initialJobState);
	// Kept in a ref as well, because the effect has to decide whether to reconnect
	// without listing the state as a dependency and tearing the stream down on
	// every log line.
	const terminal = useRef(false);
	const [attempt, setAttempt] = useState(0);
	// Which job the two above describe. Without it they outlive the job that set
	// them: roll out once, watch it finish, roll out again, and the second stream
	// never opens because `terminal` is still true from the first.
	//
	// Initialised to the first name, so a job handed in by the server is not
	// cleared on mount before anything has looked at it.
	const followed = useRef(name);

	useEffect(() => {
		if (!name) return;

		// Inside the effect, not during render: mutating a ref while rendering is
		// how a component ends up not re-rendering when it should.
		if (followed.current !== name) {
			followed.current = name;
			terminal.current = false;
			apply({ type: "reset" });
		}
		if (terminal.current) return;

		const controller = new AbortController();
		let retry: ReturnType<typeof setTimeout> | undefined;

		async function follow() {
			try {
				const response = await fetch(
					`/api/helm/job-events?job=${encodeURIComponent(name!)}`,
					{ signal: controller.signal, cache: "no-store" },
				);
				if (!response.ok || !response.body) throw new Error(`the API answered ${response.status}`);

				const { ended } = await readJobStream(response.body, (event) => {
					// A snapshot means this connection works. Clearing the backoff
					// here stops one bad start making every later reconnect wait
					// the maximum.
					if (event.type === "snapshot") setAttempt(0);
					if (event.type === "done") terminal.current = true;
					apply(event);
				});
				if (ended) return;
				throw new Error("the connection ended before the operation did");
			} catch (err) {
				// An abort is how this normally ends — the operator navigated away —
				// so it is not a failure and must not schedule a reconnect.
				if (controller.signal.aborted) return;
				apply({ type: "error", error: (err as Error).message });
				retry = setTimeout(
					() => setAttempt((n) => n + 1),
					Math.min(FIRST_RETRY_MS * 2 ** attempt, MAX_RETRY_MS),
				);
			}
		}

		void follow();

		return () => {
			// The load-bearing line. Aborting drops the connection to the BFF, which
			// drops its connection to the API, which cancels the watch at the API
			// server. Without it a closed tab leaks a goroutine and a watch per job
			// anybody ever looked at.
			controller.abort();
			if (retry !== undefined) clearTimeout(retry);
		};
	}, [name, attempt]);

	return state;
}
