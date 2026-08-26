import { forwardRef, type SelectHTMLAttributes } from "react";
import { cn } from "./cn";

/** Matches `inputClass`, at the denser padding filter toolbars use. */
export const selectClass =
	"rounded-control border border-border bg-background px-3 py-1.5 text-sm text-foreground focus:border-brand focus:outline-none focus:ring-1 focus:ring-brand disabled:opacity-50";

export type SelectProps = SelectHTMLAttributes<HTMLSelectElement>;

/** Brand-styled select. Merges `className` so per-field tweaks still apply. */
export const Select = forwardRef<HTMLSelectElement, SelectProps>(function Select(
	{ className, children, ...rest },
	ref,
) {
	return (
		<select ref={ref} className={cn(selectClass, className)} {...rest}>
			{children}
		</select>
	);
});
