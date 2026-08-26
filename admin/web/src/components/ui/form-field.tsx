import type { ReactNode } from "react";
import { Label } from "./label";

export interface FormFieldProps {
	label: string;
	htmlFor?: string;
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
