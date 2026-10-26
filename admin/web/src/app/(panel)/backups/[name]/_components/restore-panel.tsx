"use client";

import { useState } from "react";
import { ArchiveRestore } from "lucide-react";
import { restoreBackup } from "@/app/actions/backups";
import { Banner, FormField, Input, Select } from "@/components/ui";
import { CreatePanel } from "../../../_components/create-panel";
import { defaultTarget, restoreRequest, sourceOf } from "@/lib/backups/restore";
import type { BackupFile, RestoreMode } from "@/lib/api/types";

/**
 * Restoring one backup: into a new database by default, or over the one it
 * came from.
 *
 * New is the default because it is the one that cannot lose anything — the live
 * database stays as it was until somebody has looked at the copy. Replace asks
 * for the database's name typed back, the way deleting a namespace does, because
 * it drops the live one first.
 */
export function RestorePanel({
	schedule,
	file,
	onClose,
}: {
	schedule: string;
	/** The backup being restored; null while the panel is closed. */
	file: BackupFile | null;
	onClose: () => void;
}) {
	const source = file ? sourceOf(file) : null;
	const [mode, setMode] = useState<RestoreMode>("new");
	const [target, setTarget] = useState("");
	const [confirm, setConfirm] = useState("");
	const [seededFor, setSeededFor] = useState<string | null>(null);

	// Seeded per file, while the panel opens on it: the default name follows the
	// backup chosen, and a form left half-filled for another one does not carry over.
	if (file && source && seededFor !== file.id) {
		setSeededFor(file.id);
		setMode("new");
		setTarget(defaultTarget(source.database, new Date()));
		setConfirm("");
	}

	function close() {
		setSeededFor(null);
		onClose();
	}

	return (
		<CreatePanel
			open={file !== null && source !== null}
			title={source ? `Restore ${source.engine}/${source.database}` : "Restore"}
			icon={ArchiveRestore}
			submitLabel={mode === "replace" ? "Replace and restore" : "Restore"}
			description={file ? `From ${file.name}. The restore runs as a job; follow it under Runs.` : undefined}
			dirty={false}
			onClose={close}
			onSubmit={async () => {
				if (!file || !source) return { ok: false, error: "choose a backup to restore" };
				const built = restoreRequest(source.database, mode, target, confirm);
				if (!built.ok) return built;
				return restoreBackup(schedule, file.id, built.request);
			}}
		>
			<FormField label="Restore into" htmlFor="restore-mode">
				<Select id="restore-mode" value={mode} onChange={(event) => setMode(event.target.value as RestoreMode)}>
					<option value="new">A new database</option>
					<option value="replace">{source ? `${source.database}, replacing it` : "The original, replacing it"}</option>
				</Select>
			</FormField>

			{mode === "new" ? (
				<FormField label="New database" htmlFor="restore-target">
					<Input
						id="restore-target"
						value={target}
						onChange={(event) => setTarget(event.target.value)}
						autoComplete="off"
						spellCheck={false}
					/>
				</FormField>
			) : (
				<>
					<Banner
						variant="warning"
						message={
							source
								? `This drops ${source.database}, ending every session on it, and restores the backup in its place. Anything written since the backup is lost.`
								: null
						}
					/>
					<FormField label={source ? `Type ${source.database} to confirm` : "Confirm"} htmlFor="restore-confirm">
						<Input
							id="restore-confirm"
							value={confirm}
							onChange={(event) => setConfirm(event.target.value)}
							autoComplete="off"
							spellCheck={false}
						/>
					</FormField>
				</>
			)}

			{source?.engine === "postgres" ? (
				<p className="text-xs text-muted-foreground">
					Roles the backup needs and this server lacks are created without a login or a password. Existing roles are left exactly as they are.
				</p>
			) : null}
		</CreatePanel>
	);
}
