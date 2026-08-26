import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { Banner } from "@/components/ui/banner";
import { EmptyState } from "@/components/ui/empty-state";
import { Table, TBody, THead, Th } from "@/components/ui/table";

/**
 * The shell every list in the panel shares: an error banner, then either the
 * empty state or one bordered table.
 *
 * It deliberately does not know what a row looks like. The directory-surface
 * contract is about the frame — one border per boundary, rows separated by
 * hairlines rather than each drawing its own, and an empty state that names the
 * action rather than pointing at where a form used to be.
 *
 * Note the absence of "use client". Nothing here needs a browser, and it must
 * stay that way: the cluster pages are Server Components and pass `empty.icon`,
 * which is a function. Marking this a Client Component would make that a
 * "functions cannot be passed to Client Components" error at render time — one
 * the build does not catch, because it only happens when the page runs.
 */
export function Directory({
	error,
	columns,
	rows,
	isEmpty,
	empty,
	minWidth,
}: {
	error: string | null;
	columns: ReactNode;
	rows: ReactNode;
	/**
	 * Passed explicitly rather than inferred from `rows`. An empty array is
	 * truthy, so testing the rows themselves renders a headed table with nothing
	 * under it and never shows the empty state at all.
	 */
	isEmpty: boolean;
	empty: { icon: LucideIcon; title: string; description?: ReactNode; action?: ReactNode };
	minWidth?: string;
}) {
	return (
		<>
			<Banner variant="error" message={error} />
			{isEmpty ? (
				<EmptyState {...empty} />
			) : (
				<Table minWidth={minWidth}>
					<THead>{columns}</THead>
					<TBody>{rows}</TBody>
				</Table>
			)}
		</>
	);
}

/** A right-aligned actions header cell, so every table spells it the same way. */
export function ActionsHeader() {
	return <Th className="w-px whitespace-nowrap text-right">Actions</Th>;
}
