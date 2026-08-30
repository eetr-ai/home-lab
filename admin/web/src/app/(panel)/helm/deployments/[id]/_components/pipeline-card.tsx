"use client";

import { useState, useSyncExternalStore } from "react";
import { Check, Copy, Terminal } from "lucide-react";
import { IconButton, SectionCard } from "@/components/ui";
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
 * It shows the request and never the credential: the key is created in eetr-auth
 * and shown once there, and a panel offering to fill one in would be a panel that
 * held one.
 */
export function PipelineCard({
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
	const url = pipelineUrl(origin, chartId);
	const curl = pipelineCurl(origin, chartId, chartVersion);

	return (
		<SectionCard title="Deploy from a pipeline" icon={Terminal}>
			<p className="mb-4 text-sm text-muted-foreground">
				A CI job rolls this deployment forward by sending its chart version to the
				address below, authenticated with an eetr-auth API key. The key is created
				in eetr-auth and shown once; see <Code>docs/deploying-from-a-pipeline.md</Code>.
			</p>

			<Field label="Endpoint" text={url} />
			<Field label="Example request" text={curl} multiline />
		</SectionCard>
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

/**
 * Copy, when the browser will allow it.
 *
 * `navigator.clipboard` exists only in a secure context, so over plain HTTP on
 * anything but localhost it is simply absent — which is a real configuration for
 * a lab panel reached by address. The button renders nothing at all then rather
 * than offering an action that silently does nothing; the text beneath it is
 * still selectable, which is the part that always works.
 */
function CopyButton({ text, label }: { text: string; label: string }) {
	const [copied, setCopied] = useState(false);
	// Read after hydration rather than during render. The server has no
	// `navigator`, so consulting it while rendering makes the first client render
	// disagree with the server's — a hydration mismatch, over a button.
	// `useSyncExternalStore` with a server snapshot of `false` is how React spells
	// "browser-only value": nothing subscribes, the client snapshot is read once
	// hydration is done, and the button appears a beat later where it can work.
	const supported = useSyncExternalStore(
		subscribeToNothing,
		() => Boolean(navigator.clipboard),
		() => false,
	);

	if (!supported) return null;

	async function copy() {
		try {
			await navigator.clipboard.writeText(text);
			setCopied(true);
			window.setTimeout(() => setCopied(false), 2000);
		} catch {
			// A denied clipboard permission is not worth a banner: the text is right
			// there to select, and an error about copying would be louder than the
			// thing it failed at.
		}
	}

	return (
		<IconButton aria-label={copied ? "Copied" : label} onClick={copy}>
			{copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
		</IconButton>
	);
}

/** Whether the clipboard exists never changes, so there is nothing to listen to. */
function subscribeToNothing(): () => void {
	return () => {};
}

/** Inline literal. Same treatment the rest of the panel gives one. */
function Code({ children }: { children: string }) {
	return <code className="rounded-chip bg-surface-sunken px-1 py-0.5 text-xs">{children}</code>;
}
