"use client";

import { useState, useTransition } from "react";
import { Play, Table2 } from "lucide-react";
import { runQuery } from "@/app/actions/postgres";
import { Banner, Button, FormField, Select, Td, Th } from "@/components/ui";
import { Directory } from "../../../_components/directory";
import { describeResult } from "@/lib/query/console";
import type { QueryResult } from "@/lib/api/types";

/**
 * A read-only SQL console.
 *
 * Nothing here inspects the statement, deliberately. The API runs it inside a
 * READ ONLY transaction that is always rolled back, so PostgreSQL itself refuses
 * every write and DDL — with its own message, which names what it refused. A
 * pattern match over the text here would suggest the safety came from the
 * browser; it does not, and comments, CTEs and dollar quoting all defeat one.
 */
export function QueryConsole({ databases }: { databases: string[] }) {
	const [database, setDatabase] = useState(databases[0] ?? "");
	const [sql, setSql] = useState("");
	const [result, setResult] = useState<QueryResult | null>(null);
	const [error, setError] = useState<string | null>(null);
	const [pending, startTransition] = useTransition();

	function run() {
		setError(null);
		startTransition(async () => {
			const answer = await runQuery(database, sql);
			if (!answer.ok) {
				setError(answer.error);
				setResult(null);
				return;
			}
			setResult(answer.data);
		});
	}

	return (
		<div className="flex flex-col gap-4">
			<div className="flex flex-wrap items-end gap-3">
				<FormField label="Database" htmlFor="query-database" className="w-56">
					<Select
						id="query-database"
						value={database}
						onChange={(event) => setDatabase(event.target.value)}
					>
						{databases.map((name) => (
							<option key={name} value={name}>
								{name}
							</option>
						))}
					</Select>
				</FormField>
				<Button
					icon={Play}
					loading={pending}
					disabled={sql.trim() === "" || database === ""}
					onClick={run}
				>
					Run
				</Button>
				{result ? (
					<span className="pb-2 text-sm text-muted-foreground">
						{describeResult(result.rows.length, result.truncated, result.elapsedMs)}
					</span>
				) : null}
			</div>

			<textarea
				aria-label="SQL"
				value={sql}
				onChange={(event) => setSql(event.target.value)}
				spellCheck={false}
				rows={6}
				placeholder="SELECT * FROM users ORDER BY created_at DESC LIMIT 20"
				className="w-full rounded-control border border-border bg-background px-3 py-2 font-mono text-sm text-foreground focus:border-brand focus:outline-none focus:ring-1 focus:ring-brand"
			/>

			<Banner variant="error" message={error} />

			{result ? (
				<Directory
					error={null}
					isEmpty={result.rows.length === 0}
					minWidth="min-w-[640px]"
					empty={{
						icon: Table2,
						title: "No rows",
						description: "The statement ran and matched nothing.",
					}}
					columns={
						<>
							{result.columns.map((column) => (
								<Th key={column}>{column}</Th>
							))}
						</>
					}
					rows={result.rows.map((row, index) => (
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
			) : null}
		</div>
	);
}
