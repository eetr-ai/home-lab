"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";

/** How often to look while something is happening. */
const INTERVAL_MS = 2_000;

/**
 * Refreshes the page while an operation on this release is still running.
 *
 * There IS a job to poll now, and JobPanel follows it. This is the other half:
 * the job reports the operation, and this reports the release.
 *
 * The two are not the same question. A release can be left pending by an
 * operation nothing on this page started, by one whose Job was evicted, or by one
 * that ran before this tab was open — and none of those has a stream to attach
 * to. The outcome is written into Helm's own storage either way, so re-reading
 * the release is what actually says where things landed.
 *
 * It stops when the release reaches a terminal status, which is what `pending`
 * going false means. It also stops while the tab is hidden: a forgotten tab
 * polling every two seconds forever is a request every two seconds forever, and
 * nobody is looking at the answer.
 *
 * There is no overall cap. A release that stays pending is a wedged release, and
 * the page says so through stuckForMinutes rather than quietly stopping and
 * leaving a stale status looking current.
 */
export function useReleasePolling(pending: boolean) {
	const router = useRouter();

	useEffect(() => {
		if (!pending) return;

		let timer: ReturnType<typeof setInterval> | undefined;

		function start() {
			if (timer !== undefined) return;
			timer = setInterval(() => router.refresh(), INTERVAL_MS);
		}

		function stop() {
			if (timer === undefined) return;
			clearInterval(timer);
			timer = undefined;
		}

		function onVisibilityChange() {
			if (document.visibilityState === "visible") {
				// Refresh once on the way back, so returning to the tab does not
				// spend two seconds showing what was true before.
				router.refresh();
				start();
			} else {
				stop();
			}
		}

		if (document.visibilityState === "visible") start();
		document.addEventListener("visibilitychange", onVisibilityChange);

		return () => {
			stop();
			document.removeEventListener("visibilitychange", onVisibilityChange);
		};
	}, [pending, router]);
}
