import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { cn } from "./cn";

export interface EmptyStateProps {
	icon: LucideIcon;
	title: string;
	description?: ReactNode;
	/** The primary action, usually the same `<Button>` as the page header's. */
	action?: ReactNode;
	className?: string;
}

/**
 * Empty list placeholder. Owns the create CTA, so copy names the action rather
 * than the layout — never "Add one above", which stops being true the moment the
 * form moves into a side panel.
 *
 * This is for a genuinely empty collection. When filters merely exclude
 * everything, render a plain muted line instead: the fix is to change the
 * filter, not to create a record.
 */
export function EmptyState({
	icon: Icon,
	title,
	description,
	action,
	className,
}: EmptyStateProps) {
	return (
		<div
			className={cn(
				"flex flex-col items-center gap-3 rounded-card border border-dashed border-border px-6 py-10 text-center",
				className,
			)}
		>
			<Icon className="h-6 w-6 text-muted-foreground" />
			<div>
				<p className="text-sm font-medium">{title}</p>
				{description ? (
					<p className="mx-auto mt-1 max-w-sm text-sm text-muted-foreground">{description}</p>
				) : null}
			</div>
			{action}
		</div>
	);
}
