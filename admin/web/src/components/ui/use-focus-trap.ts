import { useEffect, useRef, type RefObject } from "react";

const FOCUSABLE_SELECTOR = [
	"a[href]",
	"button:not([disabled])",
	"input:not([disabled])",
	"select:not([disabled])",
	"textarea:not([disabled])",
	'[tabindex]:not([tabindex="-1"])',
].join(",");

function focusableWithin(root: HTMLElement): HTMLElement[] {
	return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)).filter(
		(el) => el.getClientRects().length > 0,
	);
}

/**
 * Traps Tab inside `ref`, moves focus in on activation, and restores it on
 * teardown. Calls `onEscape` for the Escape key.
 *
 * The listener is attached to the container node rather than to `document`, so
 * a nested overlay rendered into its own portal never reaches this handler —
 * each overlay only sees keys pressed inside its own subtree.
 *
 * The container is expected to carry `tabIndex={-1}` so it can hold focus when
 * it has no focusable children.
 */
export function useFocusTrap(
	ref: RefObject<HTMLElement | null>,
	active: boolean,
	onEscape: () => void,
) {
	// Kept in a ref so changing the handler does not tear down the trap and
	// re-run the focus-in/focus-restore effect. Written in an effect rather than
	// during render: a ref is not render output, and mutating one while rendering
	// is unsound under concurrent rendering. The handler is only ever read from a
	// keydown listener, which cannot run before the effect has committed.
	const escapeRef = useRef(onEscape);
	useEffect(() => {
		escapeRef.current = onEscape;
	}, [onEscape]);

	useEffect(() => {
		const root = ref.current;
		if (!active || !root) return;

		const previous = document.activeElement as HTMLElement | null;
		const initial =
			root.querySelector<HTMLElement>("[data-autofocus]") ?? focusableWithin(root)[0] ?? root;
		// preventScroll: the overlay is fixed and the body is locked, so letting
		// the browser scroll focus into view would jitter the page behind it.
		initial.focus({ preventScroll: true });

		const handleKeyDown = (event: KeyboardEvent) => {
			if (event.key === "Escape") {
				event.stopPropagation();
				escapeRef.current();
				return;
			}
			if (event.key !== "Tab") return;

			// Re-queried per keypress: the focusable set changes as fields
			// disable themselves during a save.
			const items = focusableWithin(root);
			if (items.length === 0) {
				event.preventDefault();
				root.focus({ preventScroll: true });
				return;
			}
			const first = items[0];
			const last = items[items.length - 1];
			const current = document.activeElement;
			if (event.shiftKey && (current === first || current === root)) {
				event.preventDefault();
				last.focus({ preventScroll: true });
			} else if (!event.shiftKey && current === last) {
				event.preventDefault();
				first.focus({ preventScroll: true });
			}
		};

		root.addEventListener("keydown", handleKeyDown);
		return () => {
			root.removeEventListener("keydown", handleKeyDown);
			// isConnected: a background refetch may have replaced the element
			// that opened the overlay while it was up.
			if (previous?.isConnected) previous.focus({ preventScroll: true });
		};
	}, [ref, active]);
}
