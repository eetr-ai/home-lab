"use client";

import { useState, useSyncExternalStore } from "react";
import { Check, Copy } from "lucide-react";
import { IconButton } from "./icon-button";

/**
 * Copy, when the browser will allow it.
 *
 * `navigator.clipboard` exists only in a secure context, so over plain HTTP on
 * anything but localhost it is simply absent — which is a real configuration for
 * a lab panel reached by address. The button renders nothing at all then rather
 * than offering an action that silently does nothing; whatever it sits beside
 * should still be selectable, which is the part that always works.
 *
 * Lifted out of the pipeline popover when the generator needed the same thing.
 * There is one clipboard in this panel and it should behave one way.
 */
export function CopyButton({ text, label }: { text: string; label: string }) {
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
			window.setTimeout(() => setCopied(false), COPIED_FOR_MS);
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

/** Long enough to read the tick, short enough not to look stuck. */
const COPIED_FOR_MS = 2000;

/** Whether the clipboard exists never changes, so there is nothing to listen to. */
function subscribeToNothing(): () => void {
	return () => {};
}
