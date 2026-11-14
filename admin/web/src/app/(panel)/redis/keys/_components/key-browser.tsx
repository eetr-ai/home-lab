"use client";

import { useRef, useState, useTransition } from "react";
import { ChevronLeft, ChevronRight, KeyRound, Trash2 } from "lucide-react";
import { deleteKey, readKey, scanKeys } from "@/app/actions/redis";
import { Button, IconButton, InlineDeleteConfirm, Td, Th } from "@/components/ui";
import { advance, currentCursor, firstPage, hasPrevious, pageNumber, retreat } from "@/lib/query/pagination";
import { displayText, formatTtl, keyId } from "@/lib/redis/text";
import type { ActionResult } from "@/lib/api/result";
import type { RedisKeyDetail, RedisKeyPage, RedisText } from "@/lib/api/redis-types";
import { ActionsHeader, Directory } from "../../../_components/directory";
import { InteractiveRow, stopRowActivation } from "../../../_components/interactive-row";
import { useRowDelete } from "../../../_components/use-row-delete";
import { KeyDetailPanel } from "./key-detail-panel";
import { PatternForm } from "./pattern-form";

const EMPTY_PAGE: RedisKeyPage = { keys: [], nextCursor: "" };

export function KeyBrowser({ pattern, first }: { pattern: string; first: ActionResult<RedisKeyPage> }) {
	const [trail, setTrail] = useState(firstPage);
	const [page, setPage] = useState(first.ok ? first.data : EMPTY_PAGE);
	const [error, setError] = useState<string | null>(first.ok ? null : first.error);
	const [paging, startPaging] = useTransition();
	const rowDelete = useRowDelete(setError);

	const [opened, setOpened] = useState<RedisText | null>(null);
	const [detail, setDetail] = useState<ActionResult<RedisKeyDetail> | null>(null);
	// Which open request is current, so a slow read of a key the operator has
	// already moved on from cannot paint over the one they are looking at.
	const request = useRef(0);

	function go(next: string[]) {
		setError(null);
		startPaging(async () => {
			const result = await scanKeys(pattern, currentCursor(next));
			if (!result.ok) {
				setError(result.error);
				return;
			}
			setTrail(next);
			setPage(result.data);
		});
	}

	async function open(key: RedisText) {
		request.current += 1;
		const mine = request.current;
		setOpened(key);
		setDetail(null);
		const result = await readKey(key);
		if (request.current === mine) setDetail(result);
	}

	// The row goes the moment the delete succeeds. A rescan would be the honest
	// refresh, but SCAN would hand back a different first page and lose the
	// operator's place for the sake of one row.
	async function remove(key: RedisText): Promise<ActionResult<void>> {
		const result = await deleteKey(key);
		if (result.ok) {
			setPage((current) => ({ ...current, keys: current.keys.filter((row) => keyId(row.key) !== keyId(key)) }));
		}
		return result;
	}

	const nothingAtAll = page.keys.length === 0 && !page.nextCursor && !hasPrevious(trail);

	return (
		<>
			<PatternForm pattern={pattern} />

			<Directory
				error={error}
				isEmpty={nothingAtAll && !pattern}
				minWidth="min-w-[640px]"
				empty={{
					icon: KeyRound,
					title: "No keys",
					description: "Redis holds nothing right now. It keeps nothing across a restart, so this is normal after one.",
				}}
				columns={
					<>
						<Th>Key</Th>
						<Th>Type</Th>
						<Th>Expires in</Th>
						<ActionsHeader />
					</>
				}
				rows={page.keys.map((row) => {
					const id = keyId(row.key);
					return (
						<InteractiveRow key={id} onActivate={() => open(row.key)}>
							<Td className="max-w-md truncate font-mono group-hover:underline">{displayText(row.key)}</Td>
							<Td className="text-muted-foreground">{row.type}</Td>
							<Td className={row.ttlMillis < 0 ? "text-muted-foreground" : ""}>{formatTtl(row.ttlMillis)}</Td>
							<Td className="text-right" onClick={stopRowActivation}>
								{rowDelete.confirmingId === id ? (
									<InlineDeleteConfirm
										label="Delete key?"
										busy={rowDelete.isDeleting(id)}
										onConfirm={() => rowDelete.confirm(id, () => remove(row.key))}
										onCancel={rowDelete.cancel}
									/>
								) : (
									<IconButton
										variant="danger"
										aria-label={`Delete ${displayText(row.key)}`}
										onClick={() => rowDelete.ask(id)}
									>
										<Trash2 className="h-4 w-4" />
									</IconButton>
								)}
							</Td>
						</InteractiveRow>
					);
				})}
			/>

			{/* SCAN promises every key once, not a full page each time: a page can
			    come back empty with more to come. Saying so is the difference
			    between "keep going" and "there is nothing here". */}
			{page.keys.length === 0 && !nothingAtAll ? (
				<p className="mt-3 text-sm text-muted-foreground">Nothing matched on this page. Keep going for the rest.</p>
			) : null}
			{nothingAtAll && pattern ? (
				<p className="mt-3 text-sm text-muted-foreground">No key matches {pattern}.</p>
			) : null}

			<div className="mt-4 flex items-center justify-end gap-2">
				<span className="mr-2 text-xs text-muted-foreground">Page {pageNumber(trail)}</span>
				<Button variant="secondary" icon={ChevronLeft} disabled={!hasPrevious(trail) || paging} onClick={() => go(retreat(trail))}>
					Previous
				</Button>
				<Button variant="secondary" icon={ChevronRight} disabled={!page.nextCursor || paging} onClick={() => go(advance(trail, page.nextCursor))}>
					Next
				</Button>
			</div>

			<KeyDetailPanel target={opened} detail={detail} onClose={() => setOpened(null)} />
		</>
	);
}
