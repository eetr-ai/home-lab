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
		<div className={cn("flex shrink-0 items-center gap-2", className)}>
			<span className="text-xs text-danger-fg">{label}</span>
			<button
				type="button"
				onClick={onConfirm}
				disabled={busy}
				className="inline-flex items-center gap-1 rounded-full border border-danger-border bg-danger-bg px-3 py-1 text-xs font-medium text-danger-fg hover:bg-danger-bg-hover disabled:opacity-50"
			>
				{busy ? (
					<Loader2 className="h-3.5 w-3.5 animate-spin" />
				) : (
					<Check className="h-3.5 w-3.5" />
				)}
				{confirmLabel}
			</button>
			<button
				type="button"
				onClick={onCancel}
				disabled={busy}
				className="inline-flex items-center gap-1 rounded-full border border-border-strong px-3 py-1 text-xs hover:bg-surface-hover disabled:opacity-50"
			>
				<X className="h-3.5 w-3.5" />
				Cancel
			</button>
		</div>
	);
}
