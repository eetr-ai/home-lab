"use client";

import type { ReactNode } from "react";
import { cn } from "@/components/ui/cn";

/**
 * A table row you can click anywhere on.
 *
 * A row that leads somewhere should be clickable across its whole width, not
 * only on whichever cell happens to hold a link. A single underlined word in one
 * column is invisible until the pointer is already on it, so the row reads as
 * inert and the way in gets missed — which is exactly what happened to the
 * releases table and the chart catalog.
 *
 * Three things make it discoverable, and all three are needed: the pointer
 * cursor, a hover background across the row, and the `group` class, which lets
 * the cell that names the thing underline itself on `group-hover` so the eye is
 * told which word is the subject.
 *
 * The row is not the accessible affordance and does not try to be. A `<tr>`
 * cannot carry button semantics cleanly, so callers keep a real `<Link>` on the
 * name (or a labelled icon button), which is what a keyboard reaches and what a
 * screen reader announces. This is a convenience for the pointer, layered over
 * something that already works without it.
 *
 * The actions cell must stop propagation, or deleting a row also opens it.
 */
export function InteractiveRow({
	onActivate,
	className,
	children,
}: {
	onActivate: () => void;
	/** Layout or selection only. The interaction classes always win. */
	className?: string;
	children: ReactNode;
}) {
	return (
		<tr
			className={cn(
				"group cursor-pointer transition-colors hover:bg-surface-hover",
				className,
			)}
			onClick={onActivate}
		>
			{children}
		</tr>
	);
}

/**
 * Wrap a row's actions cell so clicking a button in it does not also activate the
 * row.
 *
 * Its own export rather than a note in a comment, because forgetting it is
 * silent: the delete confirm opens and the row navigates away underneath it.
 */
export function stopRowActivation(event: { stopPropagation: () => void }) {
	event.stopPropagation();
}
