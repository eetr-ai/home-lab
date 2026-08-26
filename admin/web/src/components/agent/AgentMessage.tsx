"use client";

import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { AlertTriangle } from "lucide-react";
import type { Segment, Turn } from "./turns";
import ThinkingSegment from "./ThinkingSegment";
import ToolsSegment from "./ToolsSegment";
import CompactionNotice from "./CompactionNotice";
import SignalNotice from "./SignalNotice";
import UserMessage from "./UserMessage";
import { MARKDOWN_COMPONENTS } from "./markdown";

/**
 * One turn in the transcript.
 *
 * The agent is told to answer in Markdown, so this renders it — which is what
 * makes a command or a table readable in a chat panel at all. `react-markdown`
 * builds React elements rather than HTML, so nothing here reaches
 * `dangerouslySetInnerHTML` and a model that emits a `<script>` tag renders it as
 * the text it is. That is the point: the answer is generated partly from content
 * other people wrote — pod logs, event messages, whatever curl fetched.
 */
export default function AgentMessage({ turn }: { turn: Turn }) {
	if (turn.role === "user") return <UserMessage turn={turn} />;

	return (
		<div className="flex flex-col gap-1.5">
			{/* Rendered in the order they happened, which is the whole reason a turn is
			    a list. Index keys are safe here because segments are only ever appended
			    — nothing is inserted, removed or reordered. */}
			{turn.segments.map((segment, i) => (
				<SegmentView
					key={i}
					segment={segment}
					streaming={turn.streaming}
					last={i === turn.segments.length - 1}
				/>
			))}

			{/* Nothing has arrived yet, so the panel is never silent between sending and
			    the first sign of work. */}
			{turn.streaming && turn.segments.length === 0 && (
				<span className="inline-block h-3 w-1.5 animate-pulse rounded-chip bg-border-strong" />
			)}

			{turn.note && (
				<p className="flex items-start gap-1.5 text-xs text-warning-fg">
					<AlertTriangle className="mt-px h-3.5 w-3.5 shrink-0" />
					{turn.note}
				</p>
			)}
		</div>
	);
}

/** One segment, rendered as whatever it is. */
function SegmentView({
	segment,
	streaming,
	last,
}: {
	segment: Segment;
	/** The run is still going. */
	streaming: boolean;
	/** Nothing has come after this one yet. */
	last: boolean;
}) {
	switch (segment.kind) {
		case "thinking":
			// "Answered" is positional rather than a property of the turn: reasoning is
			// the main event only while nothing has followed it, which is exactly what
			// being the last segment means.
			return <ThinkingSegment text={segment.text} streaming={streaming} answered={!last} />;

		case "tools":
			return <ToolsSegment runs={segment.runs} />;

		case "compaction":
			return <CompactionNotice done={segment.done} dropped={segment.dropped} />;

		case "signal":
			return <SignalNotice signal={segment.signal} text={segment.text} />;

		case "text":
			return (
				<div className="text-sm text-foreground">
					<Markdown remarkPlugins={[remarkGfm]} components={MARKDOWN_COMPONENTS}>
						{segment.text}
					</Markdown>
				</div>
			);
	}
}
