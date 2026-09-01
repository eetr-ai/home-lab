import { useId, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { X, type LucideIcon } from "lucide-react";
import { cn } from "./cn";
import { IconButton } from "./icon-button";
import { useFocusTrap } from "./use-focus-trap";
import { usePresence } from "./use-presence";
import { useScrollLock } from "./use-scroll-lock";

/** Keep in sync with the duration-300 classes below. */
const DURATION_MS = 300;

export type SidePanelWidth = "sm" | "lg";

const panelWidth: Record<SidePanelWidth, string> = {
	sm: "sm:w-[480px]",
	lg: "sm:w-[720px]",
};

export interface SidePanelProps {
	open: boolean;
	/**
	 * Fired by the close button, the scrim, and Escape. The panel never closes
	 * itself — that is what lets the caller interpose an unsaved-changes guard.
	 */
	onRequestClose: () => void;
	title: string;
	icon?: LucideIcon;
	description?: ReactNode;
	width?: SidePanelWidth;
	/** Pinned action row. Its submit button reaches a form in `children` via `form={id}`. */
	footer?: ReactNode;
	children: ReactNode;
}

/**
 * Right-hand slide-in panel for multi-field create/edit forms. The listing stays
 * on the page behind it.
 *
 * Single-field entities should keep a compact inline add-row instead — a
 * full-screen overlay to capture one text input costs more screen than it saves.
 *
 * Note the panel is animated, and therefore a transformed element, so it forms a
 * containing block: anything `position: fixed` inside `children` resolves
 * against the panel rather than the viewport. Nested overlays (e.g.
 * `ConfirmDialog`) must render into their own portal as a *sibling* of this
 * component, never inside its children.
 */
export function SidePanel({
	open,
	onRequestClose,
	title,
	icon: Icon,
	description,
	width = "sm",
	footer,
	children,
}: SidePanelProps) {
	const panelRef = useRef<HTMLDivElement>(null);
	const titleId = useId();
	const { mounted } = usePresence(open, DURATION_MS);

	useScrollLock(mounted);
	useFocusTrap(panelRef, mounted, onRequestClose);

	// What the panel shows while it slides out is what it showed when it was open.
	//
	// Closing one of these is two things at once: the parent sets `open` to false,
	// and the form that owns the fields resets them -- every caller's onClose does
	// both, because the page's "stop showing this" and the form's "forget what was
	// typed" are the same handler. This component stays mounted for the length of
	// the exit animation, so without a snapshot the operator watches the form empty
	// itself on the way out: fields blank, a banner vanishes, a button changes
	// label. That is the flicker, and it is in all eleven panels at once.
	//
	// The rule below is suppressed rather than worked around, so here is the case
	// for it. It exists because a ref read during render can tear under concurrent
	// rendering: two renders of the same commit disagree. This ref is written only
	// while `open` and read only while closing, and the two never happen in the
	// same render -- so every render that reads it sees the same value, and the
	// worst failure available is one frame of slightly stale content inside an
	// animation whose whole job is to show content that is on its way out.
	//
	// The alternative is threading an onReset through all eleven callers and
	// splitting each one's onClose in two. That is a real option if this ever
	// needs to be more than a snapshot; it is not worth eleven touched files today.
	//
	const lastShown = useRef({ title, Icon, description, footer, children });
	// eslint-disable-next-line react-hooks/refs -- written open, read closing; see above
	if (open) lastShown.current = { title, Icon, description, footer, children };
	// eslint-disable-next-line react-hooks/refs -- same
	const shown = open ? { title, Icon, description, footer, children } : lastShown.current;

	if (!mounted) return null;

	return createPortal(
		<div className="fixed inset-0 z-50">
			{/* preventDefault on mousedown keeps focus inside the panel, so Escape
			    still reaches the trap's keydown listener after a scrim click. */}
			<div
				aria-hidden="true"
				onMouseDown={(event) => event.preventDefault()}
				onClick={onRequestClose}
				className={cn(
					"absolute inset-0 bg-scrim duration-300 motion-reduce:animate-none",
					open ? "animate-in fade-in" : "animate-out fade-out",
				)}
			/>
			<div
				ref={panelRef}
				role="dialog"
				aria-modal="true"
				aria-labelledby={titleId}
				tabIndex={-1}
				className={cn(
					"absolute inset-y-0 right-0 flex w-full flex-col border-l border-border bg-surface text-foreground shadow-xl outline-none duration-300 ease-panel motion-reduce:animate-none",
					panelWidth[width],
					open ? "animate-in slide-in-from-right" : "animate-out slide-out-to-right",
				)}
			>
				<div className="flex items-start justify-between gap-3 border-b border-border p-6 pb-4">
					<div className="min-w-0">
						<h2 id={titleId} className="flex items-center gap-2 text-lg font-medium">
							{shown.Icon ? <shown.Icon className="h-5 w-5 shrink-0" /> : null}
							{shown.title}
						</h2>
						{shown.description ? (
							<p className="mt-1 text-sm text-muted-foreground">{shown.description}</p>
						) : null}
					</div>
					<IconButton type="button" aria-label="Close panel" onClick={onRequestClose}>
						<X className="h-4 w-4" />
					</IconButton>
				</div>

				<div className="min-h-0 flex-1 overflow-y-auto p-6">{shown.children}</div>

				{shown.footer ? (
					<div className="border-t border-border p-6 pt-4">{shown.footer}</div>
				) : null}
			</div>
		</div>,
		document.body,
	);
}
