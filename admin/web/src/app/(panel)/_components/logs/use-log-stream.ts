"use client";

import { useEffect, useState } from "react";
import { appendBounded, logStreamUrl, splitLines } from "@/lib/logs/stream";

export type LogStatus = "connecting" | "streaming" | "ended" | "error";

export interface LogRequest {
	namespace: string;
	pod: string;
	container?: string;
	follow: boolean;
	tail: number;
	previous: boolean;
}

/**
 * Tail one pod's log over the streaming route.
 *
 * The AbortController is the whole design. Aborting cancels the fetch, which
 * drops the connection to the BFF, which cancels its upstream request, which
 * tears the stream down at the API server — so closing the panel actually stops
 * the work rather than leaving it running unread. It fires on unmount and
 * whenever the request changes, which is also how the buffer resets: the caller
 * keys the panel by pod, so a different pod is a different component instance.
 */
export function useLogStream(request: LogRequest) {
	const [lines, setLines] = useState<string[]>([]);
	const [status, setStatus] = useState<LogStatus>("connecting");
	const [error, setError] = useState<string | null>(null);

	const { namespace, pod, container, follow, tail, previous } = request;
	const key = `${namespace}/${pod}/${container ?? ""}/${follow}/${tail}/${previous}`;

	// Reset during render rather than in the effect below. React blesses this
	// shape for "state that depends on a prop": it re-renders immediately with the
	// new state and never commits the stale buffer, where a setState in the effect
	// would paint the previous pod's lines for one frame — and is a cascading
	// render the lint rules refuse.
	const [streamed, setStreamed] = useState(key);
	if (streamed !== key) {
		setStreamed(key);
		setLines([]);
		setError(null);
		setStatus("connecting");
	}

	useEffect(() => {
		const controller = new AbortController();
		let cancelled = false;

		const append = (parts: string[]) => {
			// `cancelled` matters as much here as on the status setters. Changing the
			// container or the previous toggle re-runs this effect on the *same*
			// component instance, and a reader.read() that resolved just before the
			// cleanup runs its continuation after the reset above cleared the buffer.
			// Without this check the old container's last chunk lands under the new
			// selection.
			if (cancelled || parts.length === 0) return;
			setLines((existing) => appendBounded(existing, parts));
		};

		void (async () => {
			try {
				const response = await fetch(
					logStreamUrl({ namespace, pod, container, follow, tail, previous }),
					{ signal: controller.signal, cache: "no-store" },
				);

				if (!response.ok || !response.body) {
					const detail = await response.text().catch(() => "");
					if (!cancelled) {
						setStatus("error");
						setError(detail || `the log stream is unavailable (${response.status})`);
					}
					return;
				}

				// Belt and braces. The proxy answers an unauthenticated /api/* with a
				// 401 rather than a redirect precisely so this cannot happen, but a
				// redirect to a sign-in page would arrive as a 200 full of HTML and
				// render as log lines. Refuse anything that is not text.
				const contentType = response.headers.get("content-type") ?? "";
				if (!contentType.startsWith("text/plain")) {
					if (!cancelled) {
						setStatus("error");
						setError("the log stream returned something that is not a log");
					}
					return;
				}

				if (!cancelled) setStatus("streaming");

				const reader = response.body.getReader();
				const decoder = new TextDecoder();
				let pending = "";
				for (;;) {
					const { done, value } = await reader.read();
					if (done) break;
					const split = splitLines(pending, decoder.decode(value, { stream: true }));
					pending = split.pending;
					append(split.lines);
				}
				// A log that does not end with a newline still has a last line.
				if (pending) append([pending]);
				if (!cancelled) setStatus("ended");
			} catch (err) {
				// An abort is how a follow stream normally ends: the operator closed
				// the panel or navigated away. Not an error to show anybody.
				if (!cancelled && (err as Error).name !== "AbortError") {
					setStatus("error");
					setError((err as Error).message);
				}
			}
		})();

		return () => {
			cancelled = true;
			controller.abort();
		};
	}, [namespace, pod, container, follow, tail, previous]);

	return { lines, status, error, clear: () => setLines([]) };
}
