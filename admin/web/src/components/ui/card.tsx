import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { cn } from "./cn";

/** `md` for forms and prose, `sm` for dense containers like list wrappers. */
export type CardPadding = "sm" | "md" | "none";

const cardPadding: Record<CardPadding, string> = {
	none: "",
	sm: "p-4",
	md: "p-6",
};

/**
 * One border per boundary: a card draws the edge, so its children must not draw
 * their own. Lists inside a card separate rows with `divide-y divide-border`,
 * never with a border per row.
 */
const cardBase = "rounded-card border border-border bg-surface";

export interface CardProps {
	children: ReactNode;
	padding?: CardPadding;
	className?: string;
}

/** Bare card wrapper. For boxes without a heading. */
export function Card({ children, padding = "md", className }: CardProps) {
	return <div className={cn(cardBase, cardPadding[padding], className)}>{children}</div>;
}

export interface SectionCardProps {
	title: string;
	icon: LucideIcon;
	children: ReactNode;
	padding?: CardPadding;
	className?: string;
}

/** Titled section card. Every heading takes a leading lucide icon at h-5 w-5. */
export function SectionCard({
	title,
	icon: Icon,
	children,
	padding = "md",
	className,
}: SectionCardProps) {
	return (
		<section className={cn(cardBase, cardPadding[padding], className)}>
			<h2 className="mb-4 flex items-center gap-2 text-lg font-medium">
				<Icon className="h-5 w-5" />
				{title}
			</h2>
			{children}
		</section>
	);
}
