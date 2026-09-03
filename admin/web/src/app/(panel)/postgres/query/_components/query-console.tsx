"use client";

import { useRef, useState, useTransition } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { executeQuery, runQuery } from "@/app/actions/postgres";
import { Banner } from "@/components/ui";
import { SqlEditor } from "@/components/editor/sql-editor";
import { classifyStatement } from "@/lib/query/classify";
import { describeResult } from "@/lib/query/console";
import type { PostgresRelation } from "@/lib/api/types";
import { ConsoleToolbar } from "./console-toolbar";
import { ResultsHeader } from "./results-header";
import { ResultsTable } from "./results-table";
import { relationKey, SchemaTree } from "./schema-tree";
import { useTableBrowse } from "./use-table-browse";

/** The page sizes the browser offers. The server clamps anything larger. */
const PAGE_SIZES = [25, 50, 100, 200] as const;

/** The unified shape a run, a browse page, and an executed statement display as. */
type Shown = {
	columns: string[];
	rows: string[][];
	truncated: boolean;
	elapsedMs: number;
	/** Present only for an executed statement: its command tag, e.g. "UPDATE 3". */
	command?: string;
};

/**
 * A SQL console with a schema tree.
 *
 * Run executes read-only (the server rolls it back); Execute commits, gated by
 * the write allowlist and behind an inline confirmation. Clicking a table fills
 * the editor with its SELECT and browses it a page at a time — keyset over the
 * primary key, so an insert or delete elsewhere cannot shift a later page (see
 * useTableBrowse). Nothing here inspects the statement: PostgreSQL runs it.
 */
export function QueryConsole({
	databases,
	selected: database,
	relations,
	relationsError,
}: {
	databases: string[];
	selected: string;
	relations: PostgresRelation[];
	relationsError: string | null;
}) {
	const router = useRouter();
	const searchParams = useSearchParams();
	const [sql, setSql] = useState("");
	const [result, setResult] = useState<Shown | null>(null);
	const [error, setError] = useState<string | null>(null);
	const [confirmingExecute, setConfirmingExecute] = useState(false);
	const [running, startRun] = useTransition();
	const [executing, startExecuting] = useTransition();
	const [switching, startSwitching] = useTransition();
	// A monotonic token for "the answer the console is still waiting for". Every
	// fetch bumps it and captures the value; a callback that finds it moved on was
	// superseded and drops its answer. One token, shared with the browse hook,
	// covers every race between running, executing, paging and switching.
	const request = useRef(0);

	const browse = useTableBrowse({ database, request, onResult: setResult, onError: setError, onSql: setSql });

	function choose(next: string) {
		// A result and a browse belong to the database they ran against; neither
		// survives the switch. Bumping the token abandons anything still in flight.
		request.current += 1;
		setResult(null);
		setError(null);
		setConfirmingExecute(false);
		browse.leave();
		const params = new URLSearchParams(searchParams.toString());
		params.set("database", next);
		startSwitching(() => router.push(`?${params.toString()}`));
	}

	const busy = running || executing || browse.paging || switching;
	const canRun = !busy && sql.trim() !== "" && database !== "";
	const willWrite = sql.trim() !== "" && classifyStatement(sql) === "write";

	// One Run for both. A read runs at once; a modifying statement stops for the
	// confirmation first — the routing is a convenience, and the server is what
	// enforces read-only or commit on each path. See lib/query/classify.
	function run() {
		if (!canRun) return;
		setError(null);
		if (willWrite) {
			setConfirmingExecute(true);
			return;
		}
		browse.leave();
		const ran = database;
		const token = (request.current += 1);
		startRun(async () => {
			const answer = await runQuery(ran, sql);
			if (token !== request.current) return;
			if (!answer.ok) {
				setError(answer.error);
				setResult(null);
				return;
			}
			setResult(answer.data);
		});
	}

	function commit() {
		setConfirmingExecute(false);
		setError(null);
		browse.leave();
		const ran = database;
		const token = (request.current += 1);
		startExecuting(async () => {
			const answer = await executeQuery(ran, sql);
			if (token !== request.current) return;
			if (!answer.ok) {
				setError(answer.error);
				return;
			}
			setResult({ ...answer.data });
			// The statement may have changed data or the schema, so re-run the server
			// page to refetch the tree it renders from.
			router.refresh();
		});
	}

	const summary = result
		? result.command
			? `${result.command} — ${result.elapsedMs} ms`
			: describeResult(result.rows.length, result.truncated, result.elapsedMs)
		: "";

	return (
		<div className="flex min-h-0 flex-1 gap-4">
			<SchemaTree
				databases={databases}
				selected={database}
				onChooseDatabase={choose}
				switching={switching}
				relations={relations}
				relationsError={relationsError}
				activeKey={browse.activeKey}
				onSelectRelation={browse.select}
			/>

			<div className="flex min-h-0 min-w-0 flex-1 flex-col gap-4 overflow-y-auto">
				<ConsoleToolbar
					context={
						browse.browsing
							? `Browsing ${relationKey(browse.browsing)}`
							: willWrite
								? "This statement modifies the database"
								: "Read-only query"
					}
					canRun={canRun}
					running={running}
					executing={executing}
					confirming={confirmingExecute}
					database={database}
					onRun={run}
					onConfirm={commit}
					onCancel={() => setConfirmingExecute(false)}
				/>

				<SqlEditor
					value={sql}
					onChange={(value) => {
						setSql(value);
						// Editing the statement withdraws a pending commit confirmation.
						if (confirmingExecute) setConfirmingExecute(false);
					}}
					onRun={run}
				/>

				<Banner variant="error" message={error} />

				{result ? (
					<>
						<ResultsHeader
							summary={summary}
							browsing={browse.browsing !== null}
							paging={browse.paging}
							pageSize={browse.pageSize}
							pageSizes={PAGE_SIZES}
							onPageSize={browse.changePageSize}
							page={browse.page}
							estimatedPages={browse.estimatedPages}
							canPrevious={browse.canPrevious}
							canNext={browse.canNext}
							onPrevious={browse.previous}
							onNext={browse.next}
						/>
						<ResultsTable
							columns={result.columns}
							rows={result.rows}
							emptyDescription={
								result.command
									? "The statement ran and returned no rows."
									: browse.browsing
										? "This table has no rows."
										: "The statement ran and matched nothing."
							}
						/>
					</>
				) : null}
			</div>
		</div>
	);
}
