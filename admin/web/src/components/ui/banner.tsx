import type { ReactNode } from "react";
import { cn } from "./cn";

export type BannerVariant = "error" | "success" | "info" | "warning";

/** Role tokens, so each variant needs one class per property instead of a
 *  light/dark pair. See src/app/theme.css. */
const bannerVariants: Record<BannerVariant, string> = {
	error: "bg-danger-bg text-danger-fg",
	success: "bg-success-bg text-success-fg",
	warning: "bg-warning-bg text-warning-fg",
	info: "bg-surface-sunken text-foreground",
};

export interface BannerProps {
	variant: BannerVariant;
	/** Renders nothing when falsy, so call sites can pass nullable state directly. */
	message: ReactNode | null | undefined;
	className?: string;
}

/** Inline status banner. Lives inside the section it relates to (no toasts). */
export function Banner({ variant, message, className }: BannerProps) {
	if (!message) return null;
	return (
		<p className={cn("mb-3 rounded-card px-3 py-2 text-sm", bannerVariants[variant], className)}>
			{message}
		</p>
	);
}
