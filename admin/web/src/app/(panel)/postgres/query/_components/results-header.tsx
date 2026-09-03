"use client";

import { ChevronLeft, ChevronRight } from "lucide-react";
import { IconButton, Select } from "@/components/ui";

/**
 * The line above the results: a summary of what came back, and — while browsing a
 * table — the page size and the pager. The page total is approximate ("~34"),
 * because it is computed from PostgreSQL's own row estimate rather than a COUNT.
 */
export function ResultsHeader({
	summary,
	browsing,
	paging,
	pageSize,
	pageSizes,
	onPageSize,
	page,
	estimatedPages,
	canPrevious,
	canNext,
	onPrevious,
	onNext,
}: {
	summary: string;
	browsing: boolean;
	paging: boolean;
	pageSize: number;
	pageSizes: readonly number[];
	onPageSize: (size: number) => void;
	page: number;
	estimatedPages: number | null;
	canPrevious: boolean;
	canNext: boolean;
	onPrevious: () => void;
	onNext: () => void;
}) {
	return (
		<div className="flex flex-wrap items-center gap-3">
			<span className="text-sm text-muted-foreground">{summary}</span>
			{browsing ? (
				<div className="ml-auto flex items-center gap-2">
					<label htmlFor="page-size" className="text-sm text-muted-foreground">
						Rows
					</label>
					<Select
						id="page-size"
						value={String(pageSize)}
						disabled={paging}
						onChange={(event) => onPageSize(Number(event.target.value))}
						className="w-20"
					>
						{pageSizes.map((size) => (
							<option key={size} value={size}>
								{size}
							</option>
						))}
					</Select>
					<div className="flex items-center gap-1">
						<IconButton aria-label="Previous page" disabled={!canPrevious} onClick={onPrevious}>
							<ChevronLeft className="h-4 w-4" />
						</IconButton>
						<span className="text-sm tabular-nums text-muted-foreground">
							Page {page}
							{estimatedPages ? ` of ~${estimatedPages}` : ""}
						</span>
						<IconButton aria-label="Next page" disabled={!canNext} onClick={onNext}>
							<ChevronRight className="h-4 w-4" />
						</IconButton>
					</div>
				</div>
			) : null}
		</div>
	);
}
