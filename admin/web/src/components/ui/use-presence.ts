import { useEffect, useState } from "react";

/**
 * Keeps a conditionally-rendered overlay mounted long enough for its exit
 * animation to play.
 *
 * Rendering on `open` directly would unmount the node the instant it closes, so
 * the exit animation never runs. Instead `mounted` trails `open` on the way down
 * by `durationMs`.
 *
 * The enter animation needs no equivalent trick: `animate-in` (tw-animate-css)
 * is a CSS *animation*, which runs from its `from` keyframe on mount. A
 * transition-based panel would additionally need a forced paint in the closed
 * state before flipping the class.
 *
 * Unmount is driven by a timer rather than `animationend` on purpose — under
 * `prefers-reduced-motion` the animation is suppressed and no `animationend`
 * would ever fire, which would strand the overlay mounted forever.
 *
 * The trade-off is that browsers clamp timers to ~1s in a hidden tab, so an
 * overlay closed just before the tab is backgrounded can linger. That is
 * deliberate: it only happens where nobody is watching the animation, whereas
 * the `animationend` failure mode strands the overlay in front of someone who
 * is. (This also makes exit timing unmeasurable in a hidden page — check
 * `document.visibilityState` before concluding the panel is slow.)
 *
 * `durationMs` MUST match the `duration-*` class on the animated node.
 */
export function usePresence(open: boolean, durationMs: number): { mounted: boolean } {
	// The exit carries a nonce rather than being a plain flag: closing again while
	// a previous exit is still running has to restart the timer, and a boolean set
	// to the value it already holds would leave the effect on the old one — which
	// then unmounts the reopened overlay part-way through its animation.
	const [exit, setExit] = useState({ id: 0, running: false });
	const [wasOpen, setWasOpen] = useState(open);

	// Adjusting state during render, which is React's own answer for deriving from
	// a changed prop. An effect would work too and would cost an extra render pass
	// on every open and close.
	if (wasOpen !== open) {
		setWasOpen(open);
		if (!open) setExit((previous) => ({ id: previous.id + 1, running: true }));
	}

	useEffect(() => {
		if (!exit.running) return;
		const timer = setTimeout(
			() => setExit((previous) => ({ ...previous, running: false })),
			durationMs,
		);
		return () => clearTimeout(timer);
	}, [exit, durationMs]);

	// Never true on the first render, so createPortal is not called during SSR or
	// the first hydration render.
	return { mounted: open || exit.running };
}
