"use client";

import { useEffect, useState } from "react";
import dynamic from "next/dynamic";
import { Bot } from "lucide-react";

/**
 * Loaded on demand, because this sits in the layout every signed-in page shares
 * and the drawer brings a Markdown renderer with it. Deploying the agent is
 * opt-in, so on an installation without one that is weight on every page for
 * something nobody can open — and even where it is deployed, it is weight before
 * anybody has asked it anything.
 */
const AgentDrawer = dynamic(() => import("./AgentDrawer"), { ssr: false });

/**
 * The drawer column and the button that opens it.
 *
 * It probes once on mount and renders nothing when the agent is not configured,
 * which is the default — a launcher that opened onto an error would be worse than
 * no launcher.
 *
 * **It pushes rather than overlays.** The column is a sibling of the page content
 * in the shell's flex row, so opening it narrows the page instead of covering it.
 * That is not decoration: the agent navigates, and watching the page change beside
 * the answer that caused it is the whole reason to have it here rather than on a
 * page of its own. It is also why there is no scrim, no focus trap and no Escape
 * handler — the page beside this is live, and treating the drawer as a dialog
 * would say the opposite.
 */
export default function AgentLauncher({ userKey }: { userKey: string }) {
	const [available, setAvailable] = useState(false);
	const [open, setOpen] = useState(false);
	// Whether the drawer has ever been opened. Separate from `open` because it
	// stays mounted once it exists, and this is what keeps it from existing at all
	// until somebody asks for it.
	const [opened, setOpened] = useState(false);

	useEffect(() => {
		const controller = new AbortController();
		fetch("/api/agent/status", { signal: controller.signal })
			.then((res) => (res.ok ? res.json() : { available: false }))
			.then((body: { available?: boolean }) => setAvailable(Boolean(body.available)))
			.catch(() => {
				// An unreachable probe means no chat, which is what the initial state
				// already says. Nothing to report on a page somebody came to for
				// something else entirely.
			});
		return () => controller.abort();
	}, []);

	if (!available) return null;

	return (
		<>
			{/* Mounted from the first open onwards and collapsed rather than unmounted,
			    so closing the drawer keeps the conversation and lets an answer in flight
			    carry on arriving — reopen and it is there, finished. Unmounting would
			    abort the fetch, which is right for a tab that has gone away and wrong
			    for a drawer somebody closed for a minute.

			    `inert` rather than `hidden`: the width animates, so the content has to
			    stay laid out while it collapses, and a zero-width column full of
			    focusable controls is a tab stop into nothing. */}
			{opened && (
				<aside
					aria-label="Sous"
					inert={!open}
					// Sticky and self-start, exactly as PanelNav is on the other side of
					// the row: without it the column stretches to the height of a long
					// page and the conversation sits at the top of it, out of view as soon
					// as anybody scrolls.
					className={`sticky top-0 h-screen max-h-dvh shrink-0 self-start overflow-hidden border-border transition-[width] duration-300 ease-panel motion-reduce:transition-none ${
						open ? "w-[min(32rem,100vw)] border-l" : "w-0"
					}`}
				>
					{/* The inner column keeps its full width while the outer one animates,
					    so the transcript does not reflow line by line on the way in. */}
					<div className="h-full w-[min(32rem,100vw)]">
						<AgentDrawer userKey={userKey} onCollapse={() => setOpen(false)} />
					</div>
				</aside>
			)}

			{!open && (
				<button
					type="button"
					onClick={() => {
						setOpened(true);
						setOpen(true);
					}}
					title="Ask Sous"
					aria-label="Ask Sous"
					className="fixed right-6 bottom-6 z-30 flex h-11 w-11 items-center justify-center rounded-full bg-brand text-brand-fg shadow-lg outline-none ring-brand transition-colors hover:bg-brand-hover focus-visible:ring-2"
				>
					<Bot className="h-5 w-5" />
				</button>
			)}
		</>
	);
}
