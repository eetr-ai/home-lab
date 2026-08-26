"use client";

import { useEffect, useRef, useState, useTransition } from "react";
import { useRouter, useSearchParams } from "next/navigation";
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
export function QueryConsole({
	databases,
	selected: database,
}: {
	databases: string[];
	selected: string;
}) {
	const router = useRouter();
	const searchParams = useSearchParams();
	const [sql, setSql] = useState("");
	const [result, setResult] = useState<QueryResult | null>(null);
	const [error, setError] = useState<string | null>(null);
	const [pending, startTransition] = useTransition();
	// A second transition, because these two are not the same wait. Sharing one
	// put the Run button in its loading state while the page was merely fetching
	// another database's name list, which says a statement is running when none is.
	const [switching, startSwitching] = useTransition();
	// Which database the console is actually showing, readable from inside a
	// callback that started before the answer arrived. A statement takes up to
	// fifteen seconds, and the selection can move while one is in flight.
	// Set both here — from the effect, which catches the back button — and
	// synchronously in `choose`, which catches the gap between a push starting and
	// the new prop arriving. An answer landing inside that gap would otherwise pass
	// the check and paint itself under a database the operator had already left.
	const shown = useRef(database);
	useEffect(() => {
		shown.current = database;
	}, [database]);

	// The choice is the address, so changing it is a navigation. The statement in
	// the box survives it: this component stays mounted across a soft navigation
	// to the same route, which is the point of putting only the scope in the URL.
	function choose(next: string) {
		// A result belongs to the database it ran against, so it does not survive the
		// switch. Left alone it is the previous database's rows sitting under the new
		// database's name — which is not a stale table, it is a wrong answer, and
		// nothing on screen would say so.
		setResult(null);
		setError(null);
		shown.current = next;
		const params = new URLSearchParams(searchParams.toString());
		params.set("database", next);
		startSwitching(() => router.push(`?${params.toString()}`));
	}

	function run() {
		setError(null);
		// The database this particular run asked about, compared against the one on
		// screen when the answer lands. A statement that started before the selection
		// moved must not paint its rows under a name they did not come from, and
		// clearing the result in `choose` does not cover it — the late completion
		// arrives afterwards.
		//
		// Checked this way rather than by disabling the selector while a query runs.
		// Disabling forbids a reasonable thing — abandoning a slow query by moving on
		// — and it would still miss the back button, which changes the selection
		// without going anywhere near `choose`.
		const ran = database;
		startTransition(async () => {
			const answer = await runQuery(ran, sql);
			if (ran !== shown.current) return;
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
						disabled={switching || databases.length === 0}
						onChange={(event) => choose(event.target.value)}
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
					disabled={switching || sql.trim() === "" || database === ""}
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
