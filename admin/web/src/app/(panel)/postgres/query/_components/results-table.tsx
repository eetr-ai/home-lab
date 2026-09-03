"use client";

import { useRef, useState } from "react";
import { Table2 } from "lucide-react";
import { CopyButton, Popover, Td, Th } from "@/components/ui";
import { Directory } from "../../../_components/directory";

/**
 * The rows a run or a browse produced, in the panel's shared table frame.
 *
 * Cells are capped so one long value cannot stretch the row off-screen, which
 * means a long value is unreadable in place — so double-clicking a cell opens a
 * popover with the whole thing in a text box and a copy button. Presentational
 * otherwise: a run and a browse page arrive the same shape and render the same.
 */
export function ResultsTable({
	columns,
	rows,
	emptyDescription,
}: {
	columns: string[];
	rows: string[][];
	emptyDescription: string;
}) {
	// The cell being read in the popover — its value and grid position, so the
	// open cell can be highlighted — and the element it is anchored to. Null when
	// the popover is closed.
	const [open, setOpen] = useState<{ value: string; row: number; column: number } | null>(null);
	const anchor = useRef<HTMLElement | null>(null);

	function openCell(cell: HTMLElement, value: string, row: number, column: number) {
		anchor.current = cell;
		setOpen({ value, row, column });
	}

	return (
		<>
			<Directory
				error={null}
				isEmpty={rows.length === 0}
				minWidth="min-w-[640px]"
				empty={{ icon: Table2, title: "No rows", description: emptyDescription }}
				columns={columns.map((column) => (
					<Th key={column} className="whitespace-nowrap">
						{column}
					</Th>
				))}
				rows={rows.map((row, index) => (
					// Index as key: a result row has no identity, and the whole table is
					// replaced on every run rather than reordered.
					<tr key={index}>
						{row.map((cellValue, at) => (
							<Td
								key={at}
								onDoubleClick={(event) => openCell(event.currentTarget, cellValue, index, at)}
								className={`align-top font-mono text-xs ${
									open?.row === index && open?.column === at ? "bg-accent-bg" : ""
								}`}
							>
								{/* NULL arrives as the literal string, which is how it is told
								    apart from an empty one once both are text. */}
								{cellValue === "NULL" ? (
									<span className="text-muted-foreground">NULL</span>
								) : (
									// A floor so a column never compresses to nothing, and a
									// ceiling with truncation so one long value does not stretch
									// the row off the screen. Double-click the cell to read it all.
									<span
										className="block min-w-[4rem] max-w-[28rem] truncate"
										title="Double-click to view"
									>
										{cellValue}
									</span>
								)}
							</Td>
						))}
					</tr>
				))}
			/>

			<Popover
				open={open !== null}
				onRequestClose={() => setOpen(null)}
				anchor={anchor}
				title="Cell value"
				width="md"
			>
				<div className="flex flex-col gap-2 p-3">
					<textarea
						readOnly
						autoFocus
						value={open?.value ?? ""}
						rows={10}
						className="w-full resize-none rounded-control border border-border bg-background px-3 py-2 font-mono text-xs text-foreground focus:outline-none"
					/>
					<div className="flex justify-end">
						<CopyButton text={open?.value ?? ""} label="Copy value" />
					</div>
				</div>
			</Popover>
		</>
	);
}
