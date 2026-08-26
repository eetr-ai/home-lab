"use client";

import { useState } from "react";
import { Check, ChevronDown, ChevronRight, Loader2, Wrench, X } from "lucide-react";
import type { ToolRun } from "./turns";

/**
 * One tool call, as a chip that opens.
 *
 * Closed it says what ran and whether it worked; open it shows the arguments the
 * model chose and what came back. That second half is the point — it is the
 * difference between believing an answer and being able to check it, and for an
 * agent holding curl inside the cluster it is the whole audit trail there is.
 */

/** Render a tool's arguments or result compactly, whatever shape they arrive in. */
function preview(value: unknown): string {
	if (value === undefined || value === null) return "";
	if (typeof value === "string") return value;
	try {
		return JSON.stringify(value, null, 2);
	} catch {
		return String(value);
	}
}

/** The one line worth showing before anything is expanded. */
function summarise(input: unknown): string {
	if (!input || typeof input !== "object") return "";
	const args = input as Record<string, unknown>;
	// A program and its argv reads as the command it is, which is what
	// run_command mostly does.
	if (typeof args.program === "string") {
		const argv = Array.isArray(args.args) ? args.args.filter((a) => typeof a === "string") : [];
		return [args.program, ...argv].join(" ");
	}
	for (const key of ["path", "name", "query"]) {
		if (typeof args[key] === "string" && args[key]) return args[key] as string;
	}
	return "";
}

export default function ToolChip({ run }: { run: ToolRun }) {
	const [open, setOpen] = useState(false);
	const detail = preview(run.input);
	const result = preview(run.output);
	const expandable = Boolean(detail || result);
	const summary = summarise(run.input);

	const tone = run.failed
		? "bg-danger-bg text-danger-fg border-danger-border"
		: run.done
			? "bg-surface-sunken text-muted-foreground border-border"
			: "bg-accent-bg text-accent-fg border-border";

	const Status = run.failed ? X : run.done ? Check : Loader2;
	const Chevron = open ? ChevronDown : ChevronRight;

	return (
		<div className={`overflow-hidden rounded-card border text-xs ${tone}`}>
			<button
				type="button"
				disabled={!expandable}
				onClick={() => setOpen((v) => !v)}
				aria-expanded={expandable ? open : undefined}
				className="flex w-full items-center gap-1.5 px-2 py-1 text-left disabled:cursor-default"
			>
				{expandable ? (
					<Chevron className="h-3 w-3 shrink-0 opacity-60" />
				) : (
					<Wrench className="h-3 w-3 shrink-0 opacity-60" />
				)}
				<span className="font-mono font-medium">{run.tool}</span>
				{summary && <span className="min-w-0 flex-1 truncate font-mono opacity-70">{summary}</span>}
				<Status
					className={`ml-auto h-3 w-3 shrink-0 ${run.done || run.failed ? "" : "animate-spin"}`}
				/>
			</button>

			{open && (
				<div className="border-t border-border px-2 py-1.5">
					{detail && <Block label="Arguments" body={detail} />}
					{result && <Block label={run.failed ? "Error" : "Result"} body={result} />}
				</div>
			)}
		</div>
	);
}

function Block({ label, body }: { label: string; body: string }) {
	return (
		<div className="mb-1.5 last:mb-0">
			<div className="mb-0.5 text-xs font-medium uppercase opacity-50">{label}</div>
			{/* Capped and scrollable: a list of every pod in a namespace should not push
			    the conversation off the screen. */}
			<pre className="max-h-32 overflow-auto rounded-control bg-surface p-1.5 font-mono text-xs leading-snug whitespace-pre-wrap">
				{body}
			</pre>
		</div>
	);
}
