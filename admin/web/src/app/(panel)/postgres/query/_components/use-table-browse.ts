"use client";

import { useState, useTransition, type RefObject } from "react";
import { browseTable } from "@/app/actions/postgres";
import { advance, currentCursor, firstPage, hasPrevious, pageNumber, retreat } from "@/lib/query/pagination";
import type { BrowseResult } from "@/lib/api/types";
import { relationKey } from "./schema-tree";

const DEFAULT_PAGE_SIZE = 100;

/** The relation currently being browsed. */
type Relation = { schema: string; name: string };

/**
 * The table-browsing half of the console's state: which relation is open, the
 * cursor trail through its pages, the page size, and the row estimate the pager
 * shows. Pulled out of the component so the lifecycle reads on its own; it shares
 * the console's request token and result/error/editor setters, so a browse and a
 * run cannot paint over each other.
 */
export function useTableBrowse(deps: {
	database: string;
	request: RefObject<number>;
	onResult: (result: BrowseResult) => void;
	onError: (error: string | null) => void;
	onSql: (sql: string) => void;
}) {
	const { database, request: requestRef, onResult, onError, onSql } = deps;
	const [browsing, setBrowsing] = useState<Relation | null>(null);
	const [trail, setTrail] = useState<string[]>(firstPage);
	const [nextCursor, setNextCursor] = useState<string | null>(null);
	const [estimatedRows, setEstimatedRows] = useState(0);
	const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
	const [paging, startPaging] = useTransition();

	function loadPage(relation: Relation, nextTrail: string[], size = pageSize) {
		const ran = database;
		const token = (requestRef.current += 1);
		startPaging(async () => {
			const answer = await browseTable(ran, {
				schema: relation.schema,
				table: relation.name,
				cursor: currentCursor(nextTrail),
				pageSize: size,
			});
			if (token !== requestRef.current) return;
			if (!answer.ok) {
				onError(answer.error);
				return;
			}
			onError(null);
			setBrowsing(relation);
			setTrail(nextTrail);
			onSql(answer.data.sql);
			setNextCursor(answer.data.nextCursor ?? null);
			setEstimatedRows(answer.data.estimatedRows);
			onResult(answer.data);
		});
	}

	return {
		browsing,
		paging,
		pageSize,
		activeKey: browsing ? relationKey(browsing) : null,
		page: pageNumber(trail),
		estimatedPages:
			browsing && estimatedRows > 0 ? Math.max(1, Math.ceil(estimatedRows / pageSize)) : null,
		canPrevious: !paging && hasPrevious(trail),
		canNext: !paging && nextCursor !== null,
		select: (relation: Relation) => loadPage(relation, firstPage()),
		previous: () => browsing && loadPage(browsing, retreat(trail)),
		next: () => browsing && loadPage(browsing, advance(trail, nextCursor ?? "")),
		changePageSize: (next: number) => {
			setPageSize(next);
			if (browsing) loadPage(browsing, firstPage(), next);
		},
		// Run and Execute call this to leave browse mode: the box is now free SQL.
		leave: () => {
			setBrowsing(null);
			setNextCursor(null);
		},
	};
}
