"use client";

import { useRef, useState, useTransition } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { ChevronLeft, ChevronRight, Play } from "lucide-react";
import { browseTable, runQuery } from "@/app/actions/postgres";
import { Banner, Button, IconButton } from "@/components/ui";
import { describeResult } from "@/lib/query/console";
import {
	advance,
	currentCursor,
	firstPage,
	hasPrevious,
	pageNumber,
	retreat,
} from "@/lib/query/pagination";
import type { PostgresRelation } from "@/lib/api/types";
import { ResultsTable } from "./results-table";
import { relationKey, SchemaTree } from "./schema-tree";

/** The unified shape a run and a browse page both display as. */
type Shown = { columns: string[]; rows: string[][]; truncated: boolean; elapsedMs: number };
/** The relation currently being browsed, or null when the box is free SQL. */
type Browsing = { schema: string; name: string } | null;

/**
 * A read-only SQL console with a schema tree.
 *
 * Two ways fill the results: typing SQL and running it (free-form, the server
 * enforces read-only), or clicking a table in the tree, which fills the editor
 * with its SELECT and browses it a page at a time. Browsing pages by the primary
 * key with a cursor, so an insert or delete elsewhere cannot shift a later page —
 * see lib/query/pagination. Editing the box and running leaves browse mode.
 *
 * Nothing here inspects the statement, deliberately: the API runs it inside a READ
 * ONLY transaction that is always rolled back, and a pattern match over the text
 * would only suggest a safety that comes from the server.
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
	const [browsing, setBrowsing] = useState<Browsing>(null);
	const [trail, setTrail] = useState<string[]>(firstPage);
	const [nextCursor, setNextCursor] = useState<string | null>(null);
	const [running, startRun] = useTransition();
	const [paging, startPaging] = useTransition();
	// A separate transition from the two above: switching database is a navigation,
	// not a statement, and sharing a pending flag would light the Run button while a
	// name list is merely being fetched.
	const [switching, startSwitching] = useTransition();
	// A monotonic token for "the answer the console is still waiting for". Every
	// action that fetches — running SQL, loading a page, switching database — bumps
	// it and captures the new value; a callback that finds the token has moved on
	// was superseded and drops its answer. A statement can take fifteen seconds, and
	// in that time the operator can run something else, page, or change database —
	// so more than one guard is needed, and comparing one token covers them all: a
	// late page cannot paint over a newer run, nor a run over a newer page.
	const request = useRef(0);

	function choose(next: string) {
		// A result, and a browse, belong to the database they ran against; neither
		// survives the switch, or it becomes the previous database's rows under the
		// new one's name. Bumping the token also abandons anything still in flight.
		request.current += 1;
		setResult(null);
		setError(null);
		setBrowsing(null);
		setNextCursor(null);
		const params = new URLSearchParams(searchParams.toString());
		params.set("database", next);
		startSwitching(() => router.push(`?${params.toString()}`));
	}

	const canRun = !running && !paging && !switching && sql.trim() !== "" && database !== "";

	function run() {
		setError(null);
		// Running free SQL leaves browse mode: the box is now whatever was typed, and
		// the pager would page a relation the statement may not even be about.
		setBrowsing(null);
		setNextCursor(null);
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

	// Fetch one page of a relation and, if nothing newer has since been asked for,
	// show it: fill the editor with the page's own SELECT, remember the trail that
	// reached it, and keep the cursor to the next page.
	function loadPage(relation: { schema: string; name: string }, nextTrail: string[]) {
		const ran = database;
		const token = (request.current += 1);
		startPaging(async () => {
			const answer = await browseTable(ran, {
				schema: relation.schema,
				table: relation.name,
				cursor: currentCursor(nextTrail),
			});
			if (token !== request.current) return;
			if (!answer.ok) {
				setError(answer.error);
				return;
			}
			setError(null);
			setBrowsing(relation);
			setTrail(nextTrail);
			setSql(answer.data.sql);
			setNextCursor(answer.data.nextCursor ?? null);
			setResult(answer.data);
		});
	}

	function selectRelation(relation: PostgresRelation) {
		loadPage({ schema: relation.schema, name: relation.name }, firstPage());
	}

	return (
		<div className="flex min-h-0 flex-1 gap-4">
			<SchemaTree
				databases={databases}
				selected={database}
				onChooseDatabase={choose}
				switching={switching}
				relations={relations}
				relationsError={relationsError}
				activeKey={browsing ? relationKey(browsing) : null}
				onSelectRelation={selectRelation}
			/>

			<div className="flex min-h-0 min-w-0 flex-1 flex-col gap-4 overflow-auto">
				<div className="flex items-center gap-3">
					<span className="text-sm text-muted-foreground">
						{browsing ? `Browsing ${relationKey(browsing)}` : "Run a read-only statement"}
					</span>
					<Button icon={Play} className="ml-auto" loading={running} disabled={!canRun} onClick={run}>
						Run
					</Button>
				</div>

				<textarea
					aria-label="SQL"
					value={sql}
					onChange={(event) => setSql(event.target.value)}
					onKeyDown={(event) => {
						// Ctrl+Enter, or Cmd+Enter on a Mac — the run shortcut every SQL
						// console shares.
						if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
							event.preventDefault();
							if (canRun) run();
						}
					}}
					spellCheck={false}
					rows={6}
					placeholder="SELECT * FROM users ORDER BY created_at DESC LIMIT 20"
					className="w-full rounded-control border border-border bg-background px-3 py-2 font-mono text-sm text-foreground focus:border-brand focus:outline-none focus:ring-1 focus:ring-brand"
				/>

				<Banner variant="error" message={error} />

				{result ? (
					<>
						<div className="flex items-center gap-3">
							<span className="text-sm text-muted-foreground">
								{describeResult(result.rows.length, result.truncated, result.elapsedMs)}
							</span>
							{browsing ? (
								<div className="ml-auto flex items-center gap-1">
									<IconButton
										aria-label="Previous page"
										disabled={paging || !hasPrevious(trail)}
										onClick={() => loadPage(browsing, retreat(trail))}
									>
										<ChevronLeft className="h-4 w-4" />
									</IconButton>
									<span className="text-sm tabular-nums text-muted-foreground">
										Page {pageNumber(trail)}
									</span>
									<IconButton
										aria-label="Next page"
										disabled={paging || nextCursor === null}
										onClick={() => loadPage(browsing, advance(trail, nextCursor ?? ""))}
									>
										<ChevronRight className="h-4 w-4" />
									</IconButton>
								</div>
							) : null}
						</div>
						<ResultsTable
							columns={result.columns}
							rows={result.rows}
							emptyDescription={
								browsing
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
