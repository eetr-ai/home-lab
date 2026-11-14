import { Check, Loader2, X } from "lucide-react";
import { cn } from "./cn";

export interface InlineDeleteConfirmProps {
	/** Prompt shown next to the confirm button, e.g. "Remove?" or "Delete user?". */
	label?: string;
	/** Confirm button text. Defaults to "Delete". */
	confirmLabel?: string;
	/** In-flight: shows a spinner on the confirm button and disables both buttons. */
	busy: boolean;
	onConfirm: () => void;
	onCancel: () => void;
	className?: string;
}

/**
 * The inline destructive-confirmation control (docs/contributing/ux-guidelines.md, "Destructive actions").
 * Presentational only — the parent owns `confirmingDeleteXId` / `deletingXId` state
 * and decides when to render this in place of the row's normal action buttons.
 *
 * It floats rather than flows. In flow, a prompt and two labelled buttons are
 * several times wider than the icons they replace, so the actions column grew
 * the moment anyone asked to delete something: the table reflowed, the prompt
 * wrapped, and the row being confirmed moved under the pointer. Now the control
 * takes exactly one icon button's footprint (the placeholder below) and the
 * confirmation hangs over the row to its left, anchored to the right edge where
 * the trash icon was — so nothing else on the page moves.
 *
 * Still inline in every sense the guidelines mean: in the row, next to what
 * triggered it, no scrim, no portal. Focus moves to Cancel because the button
 * that opened it is gone, and Escape cancels, so a keyboard user is neither
 * stranded nor one Enter away from deleting.
 */
export function InlineDeleteConfirm({
	label = "Delete?",
	confirmLabel = "Delete",
	busy,
	onConfirm,
	onCancel,
	className,
}: InlineDeleteConfirmProps) {
	return (
		<span className={cn("relative inline-flex h-7 w-7 shrink-0 align-middle", className)}>
			<span
				role="group"
				aria-label={label}
				onKeyDown={(event) => {
					if (event.key === "Escape" && !busy) onCancel();
				}}
				className="absolute top-1/2 right-0 z-10 flex -translate-y-1/2 items-center gap-2 rounded-full border border-border bg-surface py-1 pr-1 pl-3 whitespace-nowrap text-foreground shadow-xl"
			>
				<span className="text-xs text-danger-fg">{label}</span>
				<button
					type="button"
					onClick={onConfirm}
					disabled={busy}
					className="inline-flex items-center gap-1 rounded-full border border-danger-border bg-danger-bg px-3 py-1 text-xs font-medium text-danger-fg hover:bg-danger-bg-hover disabled:opacity-50"
				>
					{busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5" />}
					{confirmLabel}
				</button>
				<button
					type="button"
					onClick={onCancel}
					disabled={busy}
					// The trigger unmounted when this appeared, taking focus with it.
					autoFocus
					className="inline-flex items-center gap-1 rounded-full border border-border-strong px-3 py-1 text-xs hover:bg-surface-hover disabled:opacity-50"
				>
					<X className="h-3.5 w-3.5" />
					Cancel
				</button>
			</span>
		</span>
	);
}
