"use client";

import { useState, useTransition } from "react";
import { FileJson, Play } from "lucide-react";
import { find } from "@/app/actions/mongo";
import { Banner, Button, FormField, Input, Select } from "@/components/ui";
import { ScopePicker } from "../../../_components/scope-picker";
import { EmptyState } from "@/components/ui/empty-state";
import { describeResult, parseDocument } from "@/lib/query/console";
import type { FindResult } from "@/lib/api/types";

/**
 * A read-only document browser.
 *
 * A find and nothing else. Aggregate can write through $out and $merge, and
 * runCommand is the whole server — neither belongs behind a button labelled as a
 * read, and adding them later should be a deliberate decision rather than a
 * widening nobody noticed.
 *
 * The filter, projection and sort are sent as documents rather than as text, so
 * nothing is parsed as syntax anywhere. The API separately refuses $where,
 * $function and $accumulator at any depth: MongoDB has no read-only mode to lean
 * on, and each of those runs JavaScript inside the database with the panel's own
 * credentials.
 */
export function FindConsole({
	databases,
	database,
	collections,
}: {
	databases: string[];
	database: string;
	collections: string[];
}) {
	const [collection, setCollection] = useState(collections[0] ?? "");
	const [filter, setFilter] = useState("");
	const [sort, setSort] = useState("");
	const [limit, setLimit] = useState("50");
	const [result, setResult] = useState<FindResult | null>(null);
	const [error, setError] = useState<string | null>(null);
	const [pending, startTransition] = useTransition();

	function run() {
		setError(null);

		const parsedFilter = parseDocument(filter, "filter");
		if ("error" in parsedFilter) {
			setError(parsedFilter.error);
			return;
		}
		const parsedSort = parseDocument(sort, "sort");
		if ("error" in parsedSort) {
			setError(parsedSort.error);
			return;
		}

		startTransition(async () => {
			const answer = await find(database, {
				collection,
				filter: parsedFilter.document,
				sort: parsedSort.document,
				limit: Number(limit) || undefined,
			});
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
			<ScopePicker label="Database" param="database" options={databases} selected={database} />

			<div className="flex flex-wrap items-end gap-3">
				<FormField label="Collection" htmlFor="find-collection" className="w-56">
					<Select
						id="find-collection"
						value={collection}
						onChange={(event) => setCollection(event.target.value)}
					>
						{collections.map((name) => (
							<option key={name} value={name}>
								{name}
							</option>
						))}
					</Select>
				</FormField>
				<FormField label="Limit" htmlFor="find-limit" className="w-28">
					<Input
						id="find-limit"
						type="number"
						min={1}
						max={200}
						value={limit}
						onChange={(event) => setLimit(event.target.value)}
					/>
				</FormField>
				<Button icon={Play} loading={pending} disabled={collection === ""} onClick={run}>
					Run
				</Button>
				{result ? (
					<span className="pb-2 text-sm text-muted-foreground">
						{describeResult(result.documents.length, result.truncated, result.elapsedMs)}
					</span>
				) : null}
			</div>

			<div className="grid gap-4 lg:grid-cols-2">
				<QueryField
					label="Filter"
					id="find-filter"
					value={filter}
					onChange={setFilter}
					placeholder='{"status": "active"}'
				/>
				<QueryField
					label="Sort"
					id="find-sort"
					value={sort}
					onChange={setSort}
					placeholder='{"createdAt": -1}'
				/>
			</div>

			<Banner variant="error" message={error} />

			{result ? <Documents documents={result.documents} /> : null}
		</div>
	);
}

function QueryField({
	label,
	id,
	value,
	onChange,
	placeholder,
}: {
	label: string;
	id: string;
	value: string;
	onChange: (value: string) => void;
	placeholder: string;
}) {
	return (
		<FormField label={label} htmlFor={id}>
			<textarea
				id={id}
				value={value}
				onChange={(event) => onChange(event.target.value)}
				spellCheck={false}
				rows={4}
				placeholder={placeholder}
				className="w-full rounded-control border border-border bg-background px-3 py-2 font-mono text-sm text-foreground focus:border-brand focus:outline-none focus:ring-1 focus:ring-brand"
			/>
		</FormField>
	);
}

function Documents({ documents }: { documents: string[] }) {
	if (documents.length === 0) {
		return (
			<EmptyState
				icon={FileJson}
				title="No documents"
				description="The query ran and matched nothing."
			/>
		);
	}
	return (
		<div className="divide-y divide-border rounded-card border border-border bg-surface">
			{documents.map((document, index) => (
				// Index as key: a result has no identity, and the whole list is
				// replaced on every run rather than reordered.
				<pre key={index} className="overflow-x-auto px-4 py-3 font-mono text-xs">
					{document}
				</pre>
			))}
		</div>
	);
}
