"use client";

import { useEffect, useRef, useState } from "react";
import { ScrollText, Trash2, X } from "lucide-react";
import { IconButton, Select } from "@/components/ui";
import { isPinnedToBottom } from "@/lib/logs/stream";
import { MAX_HEIGHT, MIN_HEIGHT, usePanelHeight, writePanelHeight } from "./panel-height";
import { useLogStream, type LogStatus } from "./use-log-stream";

/**
 * A docked, resizable panel that live-tails one pod's log.
 *
 * Ported from octo's PodLogPanel, which had already worked out the parts that are
 * easy to get wrong: the abort-on-unmount that tears the stream down all the way
 * to the cluster, the bounded buffer, and the pinned-autoscroll heuristic. What
 * changed here is the theme tokens (raw Tailwind ramps fail this repo's
 * check-theme lint), a container picker (a home-lab pod is arbitrary, where
 * octo's always had one known container), and the previous-instance toggle.
 *
 * Remounting is the reset: callers key it by pod, so selecting a different pod
 * produces a new instance with an empty buffer and a fresh stream.
 */

const DEFAULT_TAIL = 200;

const STATUS_TEXT: Record<LogStatus, string> = {
	connecting: "connecting…",
	streaming: "tailing",
	ended: "stream ended",
	error: "error",
};

const STATUS_DOT: Record<LogStatus, string> = {
	connecting: "bg-muted-foreground",
	streaming: "bg-success-icon",
	ended: "bg-muted-foreground",
	error: "bg-danger-icon",
};

export function LogPanel({
	namespace,
	pod,
	containers,
	onClose,
}: {
	namespace: string;
	pod: string;
	/** The pod's containers. The API requires one by name when there is more than one. */
	containers: string[];
	onClose: () => void;
}) {
	const [container, setContainer] = useState(containers[0] ?? "");
	const [previous, setPrevious] = useState(false);
	const storedHeight = usePanelHeight();
	// Only while a drag is in flight. Committing every pointer move to storage
	// would write it a hundred times a second.
	const [draggedHeight, setDraggedHeight] = useState<number | null>(null);
	const height = draggedHeight ?? storedHeight;
	const scrollRef = useRef<HTMLDivElement>(null);
	const pinnedRef = useRef(true);

	const { lines, status, error, clear } = useLogStream({
		namespace,
		pod,
		container: container || undefined,
		// A terminated instance's log is complete; there is nothing to follow.
		follow: !previous,
		tail: DEFAULT_TAIL,
		previous,
	});

	useEffect(() => {
		const element = scrollRef.current;
		if (element && pinnedRef.current) element.scrollTop = element.scrollHeight;
	}, [lines]);

	function onScroll() {
		const element = scrollRef.current;
		if (!element) return;
		pinnedRef.current = isPinnedToBottom(
			element.scrollHeight,
			element.scrollTop,
			element.clientHeight,
		);
	}

	function startResize(event: React.PointerEvent) {
		event.preventDefault();
		const startY = event.clientY;
		const startHeight = height;
		let latest = startHeight;

		const onMove = (moved: PointerEvent) => {
			// Dragging up grows the panel, which is the direction it opens from.
			latest = Math.min(MAX_HEIGHT, Math.max(MIN_HEIGHT, startHeight + (startY - moved.clientY)));
			setDraggedHeight(latest);
		};
		// pointercancel as well as pointerup: a touch drag interrupted by a system
		// gesture, or a pointer that leaves the browser, never produces an up. The
		// move listener would then stay attached for the life of the panel and the
		// height would keep following a pointer with no button held.
		const onEnd = () => {
			window.removeEventListener("pointermove", onMove);
			window.removeEventListener("pointerup", onEnd);
			window.removeEventListener("pointercancel", onEnd);
			writePanelHeight(latest);
			// Hand control back to the store, which now holds the same number.
			setDraggedHeight(null);
		};
		window.addEventListener("pointermove", onMove);
		window.addEventListener("pointerup", onEnd);
		window.addEventListener("pointercancel", onEnd);
	}

	return (
		<section
			style={{ height }}
			className="relative mt-6 flex shrink-0 flex-col rounded-card border border-border bg-surface-sunken"
		>
			<div
				role="separator"
				aria-orientation="horizontal"
				aria-label="Resize the log panel"
				onPointerDown={startResize}
				className="absolute inset-x-0 top-0 h-1.5 -translate-y-1/2 cursor-row-resize"
			/>

			<div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-border px-3 py-2">
				<ScrollText className="h-4 w-4 shrink-0 text-muted-foreground" />
				<span aria-hidden className={`h-2 w-2 shrink-0 rounded-chip ${STATUS_DOT[status]}`} />
				<span className="truncate font-mono text-xs">{pod}</span>
				<span className="text-xs text-muted-foreground">{STATUS_TEXT[status]}</span>

				{containers.length > 1 ? (
					<Select
						aria-label="Container"
						value={container}
						onChange={(event) => setContainer(event.target.value)}
						className="h-7 py-0 text-xs"
					>
						{containers.map((name) => (
							<option key={name} value={name}>
								{name}
							</option>
						))}
					</Select>
				) : null}

				<label className="flex items-center gap-1.5 text-xs text-muted-foreground">
					<input
						type="checkbox"
						checked={previous}
						onChange={(event) => setPrevious(event.target.checked)}
					/>
					{/* Where the reason for a CrashLoopBackOff actually lives: the
					    running instance has not failed yet. */}
					Previous instance
				</label>

				<div className="ml-auto flex items-center gap-1">
					<IconButton aria-label="Clear the log" title="Clear" onClick={clear}>
						<Trash2 className="h-4 w-4" />
					</IconButton>
					<IconButton aria-label="Close the log panel" title="Close" onClick={onClose}>
						<X className="h-4 w-4" />
					</IconButton>
				</div>
			</div>

			<div
				ref={scrollRef}
				onScroll={onScroll}
				className="flex-1 overflow-auto px-3 py-2 font-mono text-xs leading-relaxed"
			>
				<LogBody lines={lines} status={status} error={error} />
			</div>
		</section>
	);
}

function LogBody({
	lines,
	status,
	error,
}: {
	lines: string[];
	status: LogStatus;
	error: string | null;
}) {
	if (lines.length === 0) {
		return (
			<p className={error ? "text-danger-fg" : "text-muted-foreground"}>
				{error ?? (status === "ended" ? "No output." : "Waiting for output…")}
			</p>
		);
	}
	// The error goes below the lines rather than instead of them. A stream can
	// fail after delivering output, and the tail already on screen is usually the
	// part that explains why — replacing it throws away the answer.
	return (
		<>
			{lines.map((line, index) => (
				// Index as key: the buffer is append-and-drop-from-the-front, lines are
				// not identities, and nothing is ever reordered or edited in place.
				<div key={index} className="whitespace-pre-wrap break-words">
					{line || " "}
				</div>
			))}
			{error ? <p className="mt-2 text-danger-fg">{error}</p> : null}
		</>
	);
}
