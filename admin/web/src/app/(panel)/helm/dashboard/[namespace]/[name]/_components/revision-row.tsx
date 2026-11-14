"use client";

import { useState, useTransition } from "react";
import { Undo2 } from "lucide-react";
import { rollbackRelease } from "@/app/actions/helm";
import { IconButton, InlineDeleteConfirm, Td } from "@/components/ui";
import { StatusBadge } from "../../../../_components/status-badge";
import { formatAge } from "@/lib/format/age";
import type { HelmRelease, HelmRevision } from "@/lib/api/types";

/**
 * One revision, with the only action a revision has.
 *
 * Rolling back is confirmed inline in the row, the same as a delete and with the
 * same control: it is destructive in the sense that matters — it changes what is
 * running — and the guidelines are explicit that a confirmation is never a
 * dialog. The shared InlineDeleteConfirm floats over the row, so the revisions
 * table does not reflow when one is asked about.
 *
 * The current revision has no rollback, because rolling back to where you already
 * are is a no-op that still creates a revision.
 */
export function RevisionRow({
	revision,
	release,
	now,
	onError,
}: {
	revision: HelmRevision;
	release: HelmRelease;
	now: Date;
	onError: (message: string | null) => void;
}) {
	const [confirming, setConfirming] = useState(false);
	const [pending, startTransition] = useTransition();

	function rollback() {
		onError(null);
		startTransition(async () => {
			const result = await rollbackRelease(release.namespace, release.name, revision.revision);
			if (!result.ok) {
				onError(result.error);
				return;
			}
			setConfirming(false);
		});
	}

	return (
		<tr>
			<Td className="text-right font-medium">{revision.revision}</Td>
			<Td>
				<StatusBadge status={revision.status} />
			</Td>
			<Td className="text-muted-foreground">{revision.chartVersion}</Td>
			<Td className="text-muted-foreground">{revision.description || "—"}</Td>
			<Td className="text-right text-muted-foreground">{formatAge(revision.updatedAt, now)}</Td>
			<Td className="text-right">
				{revision.revision === release.revision ? (
					<span className="text-xs text-muted-foreground">current</span>
				) : confirming ? (
					<InlineDeleteConfirm
						label={`Roll back to ${revision.revision}?`}
						confirmLabel="Roll back"
						busy={pending}
						onConfirm={rollback}
						onCancel={() => setConfirming(false)}
					/>
				) : (
					<IconButton
						aria-label={`Roll back to revision ${revision.revision}`}
						onClick={() => setConfirming(true)}
					>
						<Undo2 className="h-4 w-4" />
					</IconButton>
				)}
			</Td>
		</tr>
	);
}
