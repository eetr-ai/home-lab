import { forwardRef, type InputHTMLAttributes } from "react";
import { cn } from "./cn";

export interface CheckboxProps
	extends Omit<InputHTMLAttributes<HTMLInputElement>, "type" | "onChange"> {
	label: string;
	/** A line under the label explaining what turning it on actually does. */
	hint?: string;
	onChange?: (checked: boolean) => void;
}

/**
 * A labelled checkbox.
 *
 * The label *wraps* the control rather than sitting beside it with `htmlFor`,
 * which is the one place this differs from FormField: a checkbox's label is also
 * its hit target, and wrapping gives that for free without an id every caller has
 * to invent. The hint is inside the label too, so clicking the explanation toggles
 * the thing it explains.
 *
 * onChange hands over the boolean rather than the event. Every caller wants the
 * checked state, and half of them would otherwise reach for `event.target.value`
 * — which on a checkbox is the string "on" whether it is ticked or not.
 */
export const Checkbox = forwardRef<HTMLInputElement, CheckboxProps>(function Checkbox(
	{ label, hint, className, onChange, ...rest },
	ref,
) {
	return (
		<label className={cn("flex cursor-pointer items-start gap-2.5", className)}>
			<input
				ref={ref}
				type="checkbox"
				className="mt-0.5 h-4 w-4 shrink-0 cursor-pointer rounded-chip border-border accent-brand focus:outline-none focus:ring-1 focus:ring-brand disabled:opacity-50"
				onChange={(event) => onChange?.(event.target.checked)}
				{...rest}
			/>
			<span className="text-sm">
				<span className="text-foreground">{label}</span>
				{hint ? <span className="mt-0.5 block text-xs text-muted-foreground">{hint}</span> : null}
			</span>
		</label>
	);
});
