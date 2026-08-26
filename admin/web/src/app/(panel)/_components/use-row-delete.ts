"use client";

import { useState, useTransition } from "react";
import type { ActionResult } from "@/lib/api/result";

/**
 * The row-level delete state a directory table needs: which row is asking for
 * confirmation, and which one is currently being deleted.
 *
 * It lives in the parent rather than in each row so only one confirmation can be
 * open at a time — two rows both saying "Delete?" is a good way to confirm the
 * wrong one. Deleting is inline, never a dialog; see
 * docs/contributing/ux-guidelines.md.
 *
 * The list is not refetched here. The action calls `revalidatePath`, and Next
 * refreshes the Server Component that rendered these rows as part of the same
 * round trip.
 */
export function useRowDelete(report: (message: string | null) => void) {
	const [confirmingId, setConfirmingId] = useState<string | null>(null);
	const [deletingId, setDeletingId] = useState<string | null>(null);
	const [pending, startTransition] = useTransition();

	function ask(id: string) {
		report(null);
		setConfirmingId(id);
	}

	function cancel() {
		setConfirmingId(null);
	}

	function confirm(id: string, action: () => Promise<ActionResult<void>>) {
		report(null);
		setDeletingId(id);
		startTransition(async () => {
			const result = await action();
			setDeletingId(null);
			if (!result.ok) {
				report(result.error);
				return;
			}
			setConfirmingId(null);
		});
	}

	return {
		confirmingId,
		/** True while this row's delete is in flight. */
		isDeleting: (id: string) => deletingId === id && pending,
		ask,
		cancel,
		confirm,
	};
}
