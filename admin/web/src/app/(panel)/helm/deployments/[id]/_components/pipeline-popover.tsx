"use client";

import { useRef, useState } from "react";
import { Terminal } from "lucide-react";
import { Button, CopyButton, Popover } from "@/components/ui";
import { pipelineCurl, pipelineUrl } from "./pipeline-snippet";

/**
 * How a pipeline deploys this deployment, ready to paste.
 *
 * It exists because the chart id is otherwise only in the address bar, and
 * copying it out of there by hand is the step people get wrong — a deploy job
 * addressed at the wrong deployment is a valid request that upgrades the wrong
 * release. Handing over the whole request instead of the id makes that harder to
 * do than to do correctly.
 *
 * Behind a button rather than a card down the page, for the same reason the
 * version history is: this is reference material somebody reads once while
 * wiring up CI, and inline it pushed the thing you came for — the values — that
 * much further from the top on every other visit.
 *
 * It shows the request and never the credential: the key is created in eetr-auth
 * and shown once there, and a panel offering to fill one in would be a panel that
 * held one.
 */
export function PipelinePopover({
	origin,
	chartId,
	chartVersion,
}: {
	/** The panel's own origin — from the server, never `window.location`. */
	origin: string;
	chartId: string;
	/** The declared version, so the snippet is a request that would work. */
	chartVersion: string;
}) {
	const [open, setOpen] = useState(false);
	const trigger = useRef<HTMLButtonElement>(null);

	const url = pipelineUrl(origin, chartId);
	const curl = pipelineCurl(origin, chartId, chartVersion);

	return (
		<>
			{/* Icon-only, and a pill rather than an `IconButton`, so it sits at the
			    same height as the version control it shares the row with. */}
			<Button
				ref={trigger}
				variant="secondary"
				icon={Terminal}
				aria-label="Deploy from a pipeline"
				aria-expanded={open}
				aria-haspopup="dialog"
				onClick={() => setOpen((wasOpen) => !wasOpen)}
			/>

			<Popover
				open={open}
				onRequestClose={() => setOpen(false)}
				anchor={trigger}
				title="Deploy from a pipeline"
				width="lg"
			>
				<div className="border-b border-border px-4 py-3">
					<h3 className="text-sm font-medium">Deploy from a pipeline</h3>
					<p className="mt-0.5 text-xs text-muted-foreground">
						A CI job rolls this deployment forward by sending its chart version to the
						address below, authenticated with an eetr-auth API key. The key is created
						in eetr-auth and shown once; see <Code>docs/deploying-from-a-pipeline.md</Code>.
					</p>
				</div>

				<div className="p-4">
					<Field label="Endpoint" text={url} />
					<Field label="Example request" text={curl} multiline />
				</div>
			</Popover>
		</>
	);
}

/** One labelled, copyable string. */
function Field({
	label,
	text,
	multiline = false,
}: {
	label: string;
	text: string;
	multiline?: boolean;
}) {
	return (
		<div className="mb-4 last:mb-0">
			<div className="mb-1.5 flex items-center justify-between gap-2">
				<span className="text-xs font-medium text-muted-foreground">{label}</span>
				<CopyButton text={text} label={`Copy the ${label.toLowerCase()}`} />
			</div>
			{/* Selectable regardless of whether the copy button can work — the text is
			    the affordance, the button is the shortcut. */}
			<pre
				className={`overflow-x-auto rounded-control bg-surface-sunken p-3 text-xs ${
					multiline ? "" : "whitespace-pre-wrap break-all"
				}`}
			>
				<code>{text}</code>
			</pre>
		</div>
	);
}

/** Inline literal. Same treatment the rest of the panel gives one. */
function Code({ children }: { children: string }) {
	return <code className="rounded-chip bg-surface-sunken px-1 py-0.5 text-xs">{children}</code>;
}
