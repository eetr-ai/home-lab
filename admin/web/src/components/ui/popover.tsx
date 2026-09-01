"use client";

import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import type { ReactNode, RefObject } from "react";
import { createPortal } from "react-dom";
import { cn } from "./cn";
import { useFocusTrap } from "./use-focus-trap";
import { usePresence } from "./use-presence";

/** Keep in sync with the duration-150 classes below. */
const DURATION_MS = 150;

/** How far the panel sits from its trigger, and from the viewport edge. */
const GAP = 8;
const MARGIN = 12;

export type PopoverWidth = "sm" | "md" | "lg";

const popoverWidth: Record<PopoverWidth, number> = { sm: 320, md: 480, lg: 720 };

export interface PopoverProps {
	open: boolean;
	/** Fired by Escape and by a click outside. The popover never closes itself. */
	onRequestClose: () => void;
	/** The element it points at. Usually the button that opened it. */
	anchor: RefObject<HTMLElement | null>;
	title: string;
	width?: PopoverWidth;
	children: ReactNode;
}

/**
 * A panel anchored to the control that opened it.
 *
 * The third overlay in this app, and the line between it and the other two is
 * worth stating so the next one lands in the right place. `SidePanel` is for
 * filling in a form; `ConfirmDialog` is for stopping to answer a question. A
 * popover is for *looking something up* without leaving the page you are on —
 * secondary detail that would push the primary content down if it lived inline,
 * and that you dismiss the moment you have read it.
 *
 * It is deliberately not modal. There is no scrim and no scroll lock, because
 * neither would be true: the page behind is still the subject, and locking it
 * would claim otherwise. Escape and a click outside both close it, which is what
 * makes that honest.
 *
 * Positioned with fixed coordinates measured from the anchor rather than with
 * absolute positioning inside it. A popover nested in the page inherits every
 * `overflow: hidden` and stacking context between it and the root — which for a
 * panel opened from inside a bordered card means it gets clipped by the card.
 * The portal escapes all of that, and the cost is recomputing on scroll and
 * resize, which is what the listeners below are for.
 */
export function Popover({
	open,
	onRequestClose,
	anchor,
	title,
	width = "md",
	children,
}: PopoverProps) {
	const panelRef = useRef<HTMLDivElement>(null);
	const titleId = useId();
	const { mounted } = usePresence(open, DURATION_MS);
	const [position, setPosition] = useState<{ top: number; left: number } | null>(null);

	useFocusTrap(panelRef, mounted, onRequestClose);

	// Placement and the listeners that keep it current live in one layout effect:
	// it runs before paint so the panel never appears in the wrong place first,
	// and defining `place` inside it is what lets it read the two refs. Hoisting
	// it into a useCallback would mean reading a ref during render, which is not
	// sound under concurrent rendering and which the compiler refuses outright.
	useLayoutEffect(() => {
		if (!mounted) return;

		function place() {
			const trigger = anchor.current;
			if (!trigger) return;

			const rect = trigger.getBoundingClientRect();
			const panelWidth = popoverWidth[width];

			// Right-aligned to the trigger, because these open from a control in
			// the top-right corner and a left-aligned panel would hang off the
			// page. Clamped to the viewport so a narrow window moves it rather
			// than scrolling the document sideways.
			const left = Math.min(
				Math.max(MARGIN, rect.right - panelWidth),
				window.innerWidth - panelWidth - MARGIN,
			);

			// Below the trigger, unless there is more room above it. Measured
			// against the panel's real height once it has one, so a tall panel
			// flips and a short one does not.
			const height = panelRef.current?.offsetHeight ?? 0;
			const below = rect.bottom + GAP;
			const fitsBelow = below + height + MARGIN <= window.innerHeight;
			const top = fitsBelow ? below : Math.max(MARGIN, rect.top - GAP - height);

			setPosition({ top, left });
		}

		place();

		// Capture, so scrolling any ancestor is seen and not just the document.
		window.addEventListener("scroll", place, true);
		window.addEventListener("resize", place);
		return () => {
			window.removeEventListener("scroll", place, true);
			window.removeEventListener("resize", place);
		};
	}, [mounted, anchor, width]);

	// Escape closes this and nothing else.
	//
	// useFocusTrap listens on its own container, which is right for a modal whose
	// focus is inside it and wrong here: opening a popover does not reliably move
	// focus into it — measured in a production build, focus stays on the trigger,
	// which is a button inside whatever opened it. So Escape never reached this
	// popover's handler at all. It reached the SidePanel underneath, and closed
	// THAT: the panel slid out while the popover sat at its fixed coordinates,
	// which is what "flickering on dismissal" looked like.
	//
	// Handled on the document in the capture phase so the topmost overlay sees it
	// first, and stopped there so it never reaches the one below. That is the
	// layering rule a nested overlay needs, and it does not depend on knowing why
	// the focus does not land — which I could not determine.
	useEffect(() => {
		if (!open) return;

		function onKeyDown(event: KeyboardEvent) {
			if (event.key !== "Escape") return;
			event.stopPropagation();
			onRequestClose();
		}

		document.addEventListener("keydown", onKeyDown, true);
		return () => document.removeEventListener("keydown", onKeyDown, true);
	}, [open, onRequestClose]);

	// A click outside closes it — but not a click on the trigger, which owns the
	// toggle. Without that exception the trigger would close and immediately
	// reopen, and the popover would look like it ignored the second click.
	useEffect(() => {
		if (!mounted) return;

		function onPointerDown(event: PointerEvent) {
			const target = event.target as Node;
			if (panelRef.current?.contains(target)) return;
			if (anchor.current?.contains(target)) return;
			onRequestClose();
		}

		document.addEventListener("pointerdown", onPointerDown);
		return () => document.removeEventListener("pointerdown", onPointerDown);
	}, [mounted, anchor, onRequestClose]);

	if (!mounted) return null;

	return createPortal(
		<div
			ref={panelRef}
			role="dialog"
			aria-modal={false}
			aria-labelledby={titleId}
			tabIndex={-1}
			style={{
				// Off-screen until measured, and hidden as well.
				//
				// `visibility: hidden` alone was the guard, and it is not enough: a
				// popover opened from inside a SidePanel is seen for a frame in the
				// top-left corner before it lands on its anchor. It reproduces in a
				// production build, so it is not StrictMode's double-invoke, and the
				// React render log shows no visible unpositioned render — which says
				// the frame being painted is not one React thinks it asked for.
				//
				// Rather than keep hunting the mechanism, put the unmeasured panel
				// somewhere a stray paint cannot be seen. Costs nothing and does not
				// depend on being right about the cause.
				top: position?.top ?? -9999,
				left: position?.left ?? -9999,
				width: popoverWidth[width],
				visibility: position ? "visible" : "hidden",
			}}
			className={cn(
				"fixed z-50 max-h-[min(70vh,32rem)] overflow-auto rounded-card border border-border bg-surface text-foreground shadow-xl outline-none duration-150 motion-reduce:animate-none",
				// The entry animation waits for the measurement.
				//
				// `visibility: hidden` above keeps an unplaced panel from being seen,
				// but it does NOT stop a CSS animation from running: the animation
				// starts when the node is inserted, at top:0 left:0, and whatever is
				// left of it plays out from there once the panel becomes visible. On a
				// first open — the one where nothing is warm and the measurement lands
				// a frame later — that is a panel that zooms in from the top-left
				// corner of the screen.
				//
				// Withholding the class until there is a position starts the animation
				// where the panel actually is. The exit animation needs no such guard:
				// by then it has been placed.
				position && open && "animate-in fade-in zoom-in-95",
				!open && "animate-out fade-out zoom-out-95",
			)}
		>
			<h2 id={titleId} className="sr-only">
				{title}
			</h2>
			{children}
		</div>,
		document.body,
	);
}
