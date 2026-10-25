import { ExternalLink, FileArchive } from "lucide-react";
import { Td, Th } from "@/components/ui/table";
import { Directory } from "../../../_components/directory";
import { formatAge } from "@/lib/format/age";
import { formatBytes } from "@/lib/format/bytes";
import type { BackupFile } from "@/lib/api/types";

/**
 * What the schedule has kept, read from the Drive folder itself rather than from
 * the runs: a file somebody deleted in Drive is gone from here too.
 */
export function FileList({
	files,
	loadError,
	now,
}: {
	files: BackupFile[];
	loadError: string | null;
	now: Date;
}) {
	const sorted = [...files].sort((a, b) => b.createdAt.localeCompare(a.createdAt));

	return (
		<>
			<h2 className="text-lg font-medium">Backups in Drive</h2>
			<Directory
				error={loadError}
				isEmpty={sorted.length === 0}
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
					</tr>
				))}
			/>
		</>
	);
}
