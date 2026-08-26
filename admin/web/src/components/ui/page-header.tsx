import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { cn } from "./cn";

export interface PageHeaderProps {
	icon: LucideIcon;
	title: string;
	description?: ReactNode;
	/** Right-aligned primary action, usually a `<Button icon={Plus}>`. */
	action?: ReactNode;
	className?: string;
}

/**
 * The title row every admin page starts with. The page title is the heading —
 * do not wrap the content below in a second card that restates it.
 */
export function PageHeader({
	icon: Icon,
	title,
	description,
	action,
	className,
}: PageHeaderProps) {
	return (
		<div
			className={cn(
				"mb-6 flex shrink-0 flex-wrap items-start justify-between gap-3",
				className,
			)}
		>
			<div className="min-w-0">
				<h1 className="flex items-center gap-2 text-xl font-semibold">
					<Icon className="h-6 w-6 shrink-0" />
					{title}
				</h1>
				{description ? (
					<p className="mt-1 text-sm text-muted-foreground">{description}</p>
				) : null}
			</div>
			{action}
		</div>
	);
}
