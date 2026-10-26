"use client";

import { useState } from "react";
import { ArchiveRestore, ExternalLink, FileArchive } from "lucide-react";
import { IconButton, Td, Th } from "@/components/ui";
import { ActionsHeader } from "../../../_components/directory";
import { Directory } from "../../../_components/directory";
import { formatAge } from "@/lib/format/age";
import { formatBytes } from "@/lib/format/bytes";
import { sourceOf } from "@/lib/backups/restore";
import type { BackupFile } from "@/lib/api/types";
import { RestorePanel } from "./restore-panel";

/**
 * What the schedule has kept, read from the Drive folder itself rather than from
 * the runs: a file somebody deleted in Drive is gone from here too.
 */
export function FileList({
	schedule,
	files,
	loadError,
	busy,
	now,
}: {
	schedule: string;
	files: BackupFile[];
	loadError: string | null;
	/** A run is going; the API refuses a restore then, so the buttons say so first. */
	busy: boolean;
	now: Date;
}) {
	const [restoring, setRestoring] = useState<BackupFile | null>(null);
	const sorted = [...files].sort((a, b) => b.createdAt.localeCompare(a.createdAt));

	return (
		<>
			<h2 className="text-lg font-medium">Backups in Drive</h2>
			<Directory
				error={loadError}
				// Empty only after a successful load: a failed one is not "none".
				isEmpty={loadError === null && sorted.length === 0}
				minWidth="min-w-[720px]"
				empty={{
					icon: FileArchive,
					title: "Nothing in the folder yet",
					description: "Backups appear here after the first successful run.",
				}}
				columns={
					<>
						<Th>File</Th>
						<Th>Database</Th>
						<Th className="w-px whitespace-nowrap text-right">Size</Th>
						<Th className="w-px whitespace-nowrap text-right">Taken</Th>
						<ActionsHeader />
					</>
				}
				rows={sorted.map((file) => (
					<tr key={file.id}>
						<Td className="font-mono text-xs">
							{file.webViewLink ? (
								<a
									href={file.webViewLink}
									target="_blank"
									rel="noreferrer"
									className="inline-flex items-center gap-1 hover:underline"
								>
									{file.name}
									<ExternalLink className="h-3 w-3" />
								</a>
							) : (
								file.name
							)}
						</Td>
						<Td className="text-muted-foreground">{file.database}</Td>
						<Td className="w-px whitespace-nowrap text-right text-muted-foreground">{formatBytes(file.size)}</Td>
						<Td className="w-px whitespace-nowrap text-right text-muted-foreground">
							{formatAge(file.createdAt, now)} ago
						</Td>
						<Td className="text-right">
							{sourceOf(file) ? (
								<IconButton
									type="button"
									aria-label={`Restore ${file.name}`}
									disabled={busy}
									onClick={() => setRestoring(file)}
								>
									<ArchiveRestore className="h-4 w-4" />
								</IconButton>
							) : null}
						</Td>
					</tr>
				))}
			/>
			<RestorePanel schedule={schedule} file={restoring} onClose={() => setRestoring(null)} />
		</>
	);
}
