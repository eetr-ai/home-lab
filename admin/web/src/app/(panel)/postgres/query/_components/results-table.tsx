import { Table2 } from "lucide-react";
import { Td, Th } from "@/components/ui";
import { Directory } from "../../../_components/directory";

/**
 * The rows a run or a browse produced, in the panel's shared table frame.
 *
 * Presentational: it is handed columns and rows and nothing else. A run and a
 * browse page are the same shape once they arrive, so they render the same way —
 * the difference between them lives in the console around it, not here.
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
	return (
		<Directory
			error={null}
			isEmpty={rows.length === 0}
			minWidth="min-w-[640px]"
			empty={{ icon: Table2, title: "No rows", description: emptyDescription }}
			columns={columns.map((column) => (
				<Th key={column}>{column}</Th>
			))}
			rows={rows.map((row, index) => (
				// Index as key: a result row has no identity, and the whole table is
				// replaced on every run rather than reordered.
				<tr key={index}>
					{row.map((value, at) => (
						<Td key={at} className="font-mono text-xs">
							{/* NULL arrives as the literal string, which is how it is told
							    apart from an empty one once both are text. */}
							{value === "NULL" ? (
								<span className="text-muted-foreground">NULL</span>
							) : (
								value
							)}
						</Td>
					))}
				</tr>
			))}
		/>
	);
}
