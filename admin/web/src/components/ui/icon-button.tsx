import type { ButtonHTMLAttributes, ReactNode } from "react";
import { Loader2 } from "lucide-react";
import { cn } from "./cn";

export type IconButtonVariant = "default" | "danger";

const iconButtonVariants: Record<IconButtonVariant, string> = {
	default:
		"rounded-full p-1.5 text-muted-foreground hover:bg-surface-hover hover:text-foreground disabled:opacity-50",
	danger:
		"rounded-full p-1.5 text-muted-foreground hover:bg-danger-bg hover:text-danger-fg disabled:opacity-50",
};

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
	variant?: IconButtonVariant;
	loading?: boolean;
	/** Required — icon-only buttons must always carry an accessible label. */
	"aria-label": string;
	/** The lucide icon node. Replaced by a spinner while `loading`. */
	children: ReactNode;
}

/** Icon-only per-row action button (edit, delete, etc.). */
export function IconButton({
	variant = "default",
	loading = false,
	disabled,
	className,
	children,
	...rest
}: IconButtonProps) {
	return (
		<button
			{...rest}
			disabled={disabled || loading}
			className={cn(iconButtonVariants[variant], className)}
		>
			{loading ? <Loader2 className="h-4 w-4 animate-spin" /> : children}
		</button>
	);
}
