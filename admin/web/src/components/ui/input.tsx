import { forwardRef, type InputHTMLAttributes } from "react";
import { cn } from "./cn";

export const inputClass =
	"w-full rounded-control border border-border bg-background px-3 py-2 text-foreground placeholder:text-muted-foreground focus:border-brand focus:outline-none focus:ring-1 focus:ring-brand disabled:opacity-50";

export type InputProps = InputHTMLAttributes<HTMLInputElement>;

/** Brand-styled text input. Merges `className` so per-field tweaks still apply. */
export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
	{ className, ...rest },
	ref,
) {
	return <input ref={ref} className={cn(inputClass, className)} {...rest} />;
});
