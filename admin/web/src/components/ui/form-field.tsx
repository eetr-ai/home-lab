import type { ReactNode } from "react";
import { Label } from "./label";

export interface FormFieldProps {
	label: string;
	/**
	 * Required, and not by oversight. The label is rendered as a *sibling* of the
	 * control rather than wrapping it, so without `htmlFor` there is nothing
	 * associating the two and a screen reader announces an unlabelled field.
	 * Making it required moves that from something review has to catch to
	 * something the build does.
	 */
	htmlFor: string;
	children: ReactNode;
	className?: string;
}

/** A label stacked above its control. */
export function FormField({ label, htmlFor, children, className }: FormFieldProps) {
	return (
		<div className={className}>
			<Label htmlFor={htmlFor}>{label}</Label>
			{children}
		</div>
	);
}
