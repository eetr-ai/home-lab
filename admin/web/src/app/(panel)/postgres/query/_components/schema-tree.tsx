"use client";

import { useState } from "react";
import { ChevronDown, ChevronRight, Eye, KeyRound, Search, Table2 } from "lucide-react";
import { Banner, Combobox, FormField, IconButton, Input, Spinner } from "@/components/ui";
import type { PostgresRelation } from "@/lib/api/types";

/** A relation's stable key across a re-fetch: schema and name together. */
export function relationKey(relation: Pick<PostgresRelation, "schema" | "name">): string {
	return `${relation.schema}.${relation.name}`;
}

/** Its label: the bare name in public, schema-qualified anywhere else. */
function relationLabel(relation: PostgresRelation): string {
	return relation.schema === "public" ? relation.name : relationKey(relation);
}

/**
 * The console's left rail: the database picker, then the tables and views in it as
 * a tree that expands to each relation's columns and their types.
 *
 * Choosing a relation is the primary action — it fills the editor and browses the
 * relation — so the whole row selects; the chevron beside it only toggles the
 * columns, and stops the click from also selecting.
 */
export function SchemaTree({
	databases,
	selected,
	onChooseDatabase,
	switching,
	relations,
	relationsError,
	activeKey,
	onSelectRelation,
}: {
	databases: string[];
	selected: string;
	onChooseDatabase: (database: string) => void;
	switching: boolean;
	relations: PostgresRelation[];
	relationsError: string | null;
	activeKey: string | null;
	onSelectRelation: (relation: PostgresRelation) => void;
}) {
	const [filter, setFilter] = useState("");
	const [filterOpen, setFilterOpen] = useState(false);
	const [expanded, setExpanded] = useState<Set<string>>(new Set());

	function toggleFilter() {
		setFilterOpen((open) => {
			// Closing clears it, so a hidden filter is never silently narrowing the tree.
			if (open) setFilter("");
			return !open;
		});
	}

	function toggle(key: string) {
		setExpanded((current) => {
			const next = new Set(current);
			if (next.has(key)) next.delete(key);
			else next.add(key);
			return next;
		});
	}

	const needle = filter.trim().toLowerCase();
	const shown = needle
		? relations.filter((relation) => relationKey(relation).toLowerCase().includes(needle))
		: relations;

	return (
		<div className="flex min-h-0 w-72 shrink-0 flex-col gap-3">
			<FormField label="Database" htmlFor="schema-database">
				<Combobox
					id="schema-database"
					label="Database"
					items={databases}
					selected={selected === "" ? null : selected}
					onSelect={onChooseDatabase}
					toKey={(name) => name}
					toText={(name) => name}
					placeholder="Select a database"
					empty="No databases."
					disabled={switching || databases.length === 0}
				/>
			</FormField>

			<div className="flex items-center justify-between px-0.5">
				<span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
					Tables
				</span>
				<IconButton
					aria-label={filterOpen ? "Hide table filter" : "Filter tables"}
					onClick={toggleFilter}
					className={filter ? "text-brand" : undefined}
				>
					<Search className="h-4 w-4" />
				</IconButton>
			</div>

			{filterOpen ? (
				<Input
					autoFocus
					aria-label="Filter tables"
					placeholder="Filter tables"
					value={filter}
					onChange={(event) => setFilter(event.target.value)}
				/>
			) : null}

			<div className="min-h-0 flex-1 overflow-y-auto rounded-card border border-border bg-surface">
				{relationsError ? (
					<div className="p-3">
						<Banner variant="error" message={relationsError} />
					</div>
				) : switching ? (
					<div className="flex items-center gap-2 p-3 text-sm text-muted-foreground">
						<Spinner /> Loading tables…
					</div>
				) : shown.length === 0 ? (
					<p className="p-3 text-sm text-muted-foreground">
						{relations.length === 0 ? "No tables or views." : "No tables match the filter."}
					</p>
				) : (
					<ul className="divide-y divide-border">
						{shown.map((relation) => (
							<RelationNode
								key={relationKey(relation)}
								relation={relation}
								active={relationKey(relation) === activeKey}
								open={expanded.has(relationKey(relation))}
								onToggle={() => toggle(relationKey(relation))}
								onSelect={() => onSelectRelation(relation)}
								label={relationLabel(relation)}
							/>
						))}
					</ul>
				)}
			</div>
		</div>
	);
}

function RelationNode({
	relation,
	active,
	open,
	onToggle,
	onSelect,
	label,
}: {
	relation: PostgresRelation;
	active: boolean;
	open: boolean;
	onToggle: () => void;
	onSelect: () => void;
	label: string;
}) {
	const Chevron = open ? ChevronDown : ChevronRight;
	const Kind = relation.kind === "table" ? Table2 : Eye;
	return (
		<li>
			<div
				className={`flex items-center gap-1 pr-2 ${active ? "bg-accent-bg" : "hover:bg-surface-hover"}`}
			>
				<button
					type="button"
					aria-label={open ? "Collapse columns" : "Expand columns"}
					onClick={onToggle}
					className="rounded-full p-1 text-muted-foreground hover:text-foreground"
				>
					<Chevron className="h-3.5 w-3.5" />
				</button>
				<button
					type="button"
					onClick={onSelect}
					title={`${relationKey(relation)} — ${relation.kind}`}
					className={`flex min-w-0 flex-1 items-center gap-2 py-1.5 text-left text-sm ${active ? "text-accent-fg" : "text-foreground"}`}
				>
					<Kind className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
					<span className="truncate font-mono">{label}</span>
				</button>
			</div>
			{open ? (
				<ul className="pb-1 pl-8 pr-2">
					{relation.columns.map((column) => (
						<li key={column.name} className="flex items-baseline gap-2 py-0.5 text-xs">
							{column.primaryKey ? (
								<KeyRound className="h-3 w-3 shrink-0 translate-y-0.5 text-warning-fg" />
							) : (
								<span className="w-3 shrink-0" />
							)}
							<span className="truncate font-mono text-foreground">{column.name}</span>
							<span className="ml-auto shrink-0 truncate font-mono text-muted-foreground">
								{column.type}
							</span>
						</li>
					))}
				</ul>
			) : null}
		</li>
	);
}
