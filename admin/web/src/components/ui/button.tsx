import type { ButtonHTMLAttributes, Ref } from "react";
import { Loader2, type LucideIcon } from "lucide-react";
import { cn } from "./cn";

export type ButtonVariant = "primary" | "secondary" | "destructiveConfirm";

/** Variant → class strings, taken verbatim from docs/contributing/ux-guidelines.md. */
export const buttonVariants: Record<ButtonVariant, string> = {
	primary:
		"rounded-full bg-brand px-5 py-2 text-sm font-medium text-brand-fg hover:bg-brand-hover disabled:opacity-50",
	secondary:
		"rounded-full border border-border-strong px-4 py-2 text-sm font-medium hover:bg-surface-hover disabled:opacity-50",
	destructiveConfirm:
		"inline-flex items-center gap-1 rounded-full border border-danger-border bg-danger-bg px-3 py-1 text-xs font-medium text-danger-fg hover:bg-danger-bg-hover disabled:opacity-50",
};

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
	/**
	 * The underlying button element. Declared as an ordinary prop, which is what
	 * React 19 made refs — no forwardRef wrapper. Needed by anything that has to
	 * measure this button, such as a Popover anchored to it.
	 */
	ref?: Ref<HTMLButtonElement>;
	variant?: ButtonVariant;
	/** When true, the leading icon is replaced by a spinner and the button is disabled. */
	loading?: boolean;
	/** Optional leading lucide icon. Adds `flex items-center gap-2` so the label aligns. */
	icon?: LucideIcon;
}

/**
 * Pill-shaped button. Pass `className` for layout-only additions (e.g. `w-full`,
 * focus rings) — variant classes always win for color/shape.
 */
export function Button({
	variant = "primary",
	loading = false,
	icon: Icon,
	disabled,
	className,
	children,
	...rest
}: ButtonProps) {
	const hasLeading = loading || Boolean(Icon);
	return (
		<button
			{...rest}
			disabled={disabled || loading}
			className={cn(
				buttonVariants[variant],
				hasLeading && "flex items-center gap-2",
				className,
			)}
		>
			{loading ? (
				<Loader2 className="h-4 w-4 animate-spin" />
			) : Icon ? (
				<Icon className="h-4 w-4" />
			) : null}
			{children}
		</button>
	);
}
