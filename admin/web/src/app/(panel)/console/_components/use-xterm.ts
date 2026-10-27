"use client";

import "@xterm/xterm/css/xterm.css";
import { type RefObject, useEffect, useState } from "react";
import { ClipboardAddon } from "@xterm/addon-clipboard";
import { FitAddon } from "@xterm/addon-fit";
import { WebglAddon } from "@xterm/addon-webgl";
import { Terminal } from "@xterm/xterm";
import { terminalTheme } from "@/lib/console/theme";

export interface Xterm {
	term: Terminal;
	fit: FitAddon;
}

/** A role's computed value on the root, where theme.css defines it. */
function readRole(role: string): string {
	return getComputedStyle(document.documentElement).getPropertyValue(role);
}

/** Geist Mono when it has loaded, then the same fallback stack the editor uses. */
function monoFont(): string {
	const geist = getComputedStyle(document.body).getPropertyValue("--font-geist-mono").trim();
	return `${geist ? `${geist}, ` : ""}ui-monospace, SFMono-Regular, Menlo, Consolas, monospace`;
}

/**
 * Call `onChange` whenever the panel's theme might have changed: the switcher
 * toggles a class on the root, and "system" follows the OS preference.
 */
function watchTheme(onChange: () => void): () => void {
	const observer = new MutationObserver(onChange);
	observer.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
	const media = matchMedia("(prefers-color-scheme: dark)");
	media.addEventListener("change", onChange);
	return () => {
		observer.disconnect();
		media.removeEventListener("change", onChange);
	};
}

/**
 * An xterm.js terminal mounted in `host`, sized to it and themed from the panel.
 * Returned as state rather than a ref so the socket hook runs once it exists.
 */
export function useXterm(host: RefObject<HTMLDivElement | null>): Xterm | null {
	const [xterm, setXterm] = useState<Xterm | null>(null);

	useEffect(() => {
		const element = host.current;
		if (!element) return;

		const term = new Terminal({
			cursorBlink: true,
			fontFamily: monoFont(),
			fontSize: 13,
			scrollback: 10_000,
			theme: terminalTheme(readRole),
			// tmux runs with the mouse on, so a plain drag belongs to tmux, not to
			// the browser. Option-drag on a Mac (Shift-drag elsewhere) is the way
			// past it to a native selection that Cmd/Ctrl-C copies.
			macOptionClickForcesSelection: true,
		});
		const fit = new FitAddon();
		term.loadAddon(fit);
		// What a plain drag in tmux selects, tmux hands to its terminal as OSC 52;
		// this is what turns that into the browser's clipboard.
		term.loadAddon(new ClipboardAddon());
		term.open(element);

		// The DOM renderer redraws through layout on every keystroke's echo, which
		// is where typing felt behind. WebGL needs the terminal in the page first,
		// and a lost GPU context drops back to the DOM renderer rather than to a
		// blank pane.
		try {
			const webgl = new WebglAddon();
			webgl.onContextLoss(() => webgl.dispose());
			term.loadAddon(webgl);
		} catch {
			// No WebGL here; the DOM renderer still works, only slower.
		}

		// A hidden tab measures as zero, and fitting to that would shrink the shell
		// to nothing; it is fitted again when it is shown.
		const refit = () => {
			if (element.offsetParent !== null) fit.fit();
		};
		refit();
		const resized = new ResizeObserver(refit);
		resized.observe(element);
		const stopWatching = watchTheme(() => {
			term.options.theme = terminalTheme(readRole);
		});

		// Handing the terminal to state is the point of this effect: it is created
		// here, against a DOM node, and the socket hook needs it once it exists.
		// eslint-disable-next-line react-hooks/set-state-in-effect
		setXterm({ term, fit });
		return () => {
			resized.disconnect();
			stopWatching();
			term.dispose();
			setXterm(null);
		};
	}, [host]);

	return xterm;
}
