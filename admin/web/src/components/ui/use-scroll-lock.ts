import { useEffect } from "react";

// Module scope, not per-hook: a side panel and the confirm dialog stacked on
// top of it both lock, and the inner one unmounting must not unlock the page
// while the outer one is still open.
let lockCount = 0;
let previousOverflow = "";
let previousPaddingRight = "";

/**
 * Locks body scroll while `active`, compensating for the scrollbar gutter so
 * the page behind the scrim does not shift sideways as it locks.
 *
 * Safe to nest — the lock is reference-counted and only the outermost release
 * restores the original styles.
 */
export function useScrollLock(active: boolean) {
	useEffect(() => {
		if (!active) return;

		if (lockCount === 0) {
			const { body } = document;
			previousOverflow = body.style.overflow;
			previousPaddingRight = body.style.paddingRight;
			// 0 on platforms with overlay scrollbars (macOS), where this is a no-op.
			const gutter = window.innerWidth - document.documentElement.clientWidth;
			body.style.overflow = "hidden";
			if (gutter > 0) body.style.paddingRight = `${gutter}px`;
		}
		lockCount += 1;

		return () => {
			lockCount -= 1;
			if (lockCount === 0) {
				document.body.style.overflow = previousOverflow;
				document.body.style.paddingRight = previousPaddingRight;
			}
		};
	}, [active]);
}
