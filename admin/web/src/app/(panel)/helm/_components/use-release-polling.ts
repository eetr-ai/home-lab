"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";

/** How often to look while something is happening. */
const INTERVAL_MS = 2_000;

/**
 * Refreshes the page while an operation on this release is still running.
 *
 * This exists because every Helm mutation answers 202. There is no job to poll
 * and no id to poll it with: the outcome is written into Helm's own storage, so
 * re-reading the release is the only way to find out what happened — and that
 * makes this page the progress indicator.
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
