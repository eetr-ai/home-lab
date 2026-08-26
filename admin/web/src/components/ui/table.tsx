import type { ReactNode, TdHTMLAttributes, ThHTMLAttributes } from "react";
import { cn } from "./cn";

export interface TableProps {
	children: ReactNode;
	/** Tailwind min-width utility for the inner table, e.g. "min-w-[640px]". */
	minWidth?: string;
	className?: string;
}

/**
 * Table chrome: one bordered container that owns the edge. Rows inside must not
 * draw their own borders — `TBody` separates them with divide-y hairlines.
 */
export function Table({ children, minWidth, className }: TableProps) {
	return (
		// tabIndex on the scroll container: a horizontally scrollable region has
		// to be reachable by keyboard, or its overflowed columns are unreachable
		// without a pointer (WCAG 2.1.1).
		<div
			tabIndex={0}
			className={cn(
				"overflow-x-auto rounded-card border border-border bg-surface",
				"focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand",
				className,
			)}
		>
			<table className={cn("w-full text-left text-sm", minWidth)}>{children}</table>
		</div>
	);
}

export interface THeadProps {
	/** The `<Th>` cells. The header `<tr>` is supplied for you. */
	children: ReactNode;
	className?: string;
}

export function THead({ children, className }: THeadProps) {
	return (
		<thead className={cn("border-b border-border bg-surface-sunken", className)}>
			<tr>{children}</tr>
		</thead>
	);
}

export interface TBodyProps {
	children: ReactNode;
	className?: string;
}

export function TBody({ children, className }: TBodyProps) {
	return <tbody className={cn("divide-y divide-border", className)}>{children}</tbody>;
}

export type ThProps = ThHTMLAttributes<HTMLTableCellElement>;

export function Th({ className, children, ...rest }: ThProps) {
	return (
		<th className={cn("px-4 py-2.5 font-medium", className)} {...rest}>
			{children}
		</th>
	);
}

export type TdProps = TdHTMLAttributes<HTMLTableCellElement>;

export function Td({ className, children, ...rest }: TdProps) {
	return (
		<td className={cn("px-4 py-2.5", className)} {...rest}>
			{children}
		</td>
	);
}
