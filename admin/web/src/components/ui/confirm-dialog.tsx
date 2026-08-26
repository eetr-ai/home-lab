import { useId, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { cn } from "./cn";
import { Button } from "./button";
import { useFocusTrap } from "./use-focus-trap";
import { usePresence } from "./use-presence";
import { useScrollLock } from "./use-scroll-lock";

/** Keep in sync with the duration-150 classes below. */
const DURATION_MS = 150;

export interface ConfirmDialogProps {
	open: boolean;
	title: string;
	description?: ReactNode;
	confirmLabel: string;
	cancelLabel?: string;
	/**
	 * Which action is visually primary and takes initial focus. Defaults to
	 * "confirm"; pass "cancel" when the safe choice should be the default — as
	 * in an unsaved-changes guard, where Enter must not discard the edits.
	 */
	emphasis?: "confirm" | "cancel";
	busy?: boolean;
	onConfirm: () => void;
	onCancel: () => void;
}

/**
 * Centered confirmation dialog.
 *
 * Deliberately narrow in scope: row-level destructive actions use the inline
 * `InlineDeleteConfirm` instead. This exists for confirming a *dismissal*,
 * where the surface an inline confirmation would attach to is the very thing
 * going away. Deleting a record is never a dialog.
 *
 * Renders into its own portal at z-60 so it stacks above `SidePanel` (z-50)
 * and escapes the panel's transform containing block.
 */
export function ConfirmDialog({
	open,
	title,
	description,
	confirmLabel,
	cancelLabel = "Cancel",
	emphasis = "confirm",
	busy = false,
	onConfirm,
	onCancel,
}: ConfirmDialogProps) {
	const dialogRef = useRef<HTMLDivElement>(null);
	const titleId = useId();
	const descriptionId = useId();
	const { mounted } = usePresence(open, DURATION_MS);

	// The dialog stays mounted for its exit animation, so `open` — not just
	// `mounted` — gates interaction: without this a click landing during those
	// 150ms would fire the handler a second time. `busy` blocks the same doors
	// while the confirmed action is in flight, when both buttons are disabled.
	const interactive = open && !busy;
	const cancelIfInteractive = () => {
		if (interactive) onCancel();
	};
	const confirmIfInteractive = () => {
		if (interactive) onConfirm();
	};

	useScrollLock(mounted);
	useFocusTrap(dialogRef, mounted, cancelIfInteractive);

	if (!mounted) return null;

	const confirmIsPrimary = emphasis === "confirm";

	return createPortal(
		<div className="fixed inset-0 z-[60] flex items-center justify-center p-6">
			<div
				aria-hidden="true"
				onMouseDown={(event) => event.preventDefault()}
				onClick={cancelIfInteractive}
				className={cn(
					"absolute inset-0 bg-scrim duration-150 motion-reduce:animate-none",
					open ? "animate-in fade-in" : "animate-out fade-out",
				)}
			/>
			<div
				ref={dialogRef}
				role="dialog"
				aria-modal="true"
				aria-labelledby={titleId}
				aria-describedby={description ? descriptionId : undefined}
				tabIndex={-1}
				className={cn(
					"relative w-full max-w-md rounded-card border border-border bg-surface p-6 text-foreground shadow-xl outline-none duration-150 motion-reduce:animate-none",
					open ? "animate-in fade-in zoom-in-95" : "animate-out fade-out zoom-out-95",
				)}
			>
				<h2 id={titleId} className="text-lg font-medium">
					{title}
				</h2>
				{description ? (
					<p id={descriptionId} className="mt-2 text-sm text-muted-foreground">
						{description}
					</p>
				) : null}
				<div className="mt-6 flex flex-wrap justify-end gap-2">
					<Button
						type="button"
						variant={confirmIsPrimary ? "secondary" : "primary"}
						onClick={cancelIfInteractive}
						disabled={!interactive}
						{...(confirmIsPrimary ? {} : { "data-autofocus": true })}
					>
						{cancelLabel}
					</Button>
					<Button
						type="button"
						variant={confirmIsPrimary ? "primary" : "secondary"}
						onClick={confirmIfInteractive}
						disabled={!interactive}
						loading={busy}
						{...(confirmIsPrimary ? { "data-autofocus": true } : {})}
					>
						{confirmLabel}
					</Button>
				</div>
			</div>
		</div>,
		document.body,
	);
}
