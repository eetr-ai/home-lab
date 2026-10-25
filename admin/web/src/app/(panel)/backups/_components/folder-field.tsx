"use client";

import { useState } from "react";
import { FolderOpen } from "lucide-react";
import { pickerToken } from "@/app/actions/backups";
import { Banner, Button, Label } from "@/components/ui";
import { pickFolder } from "@/lib/backups/picker";
import type { BackupFolder, GoogleStatus } from "@/lib/api/types";

/**
 * The Drive folder, chosen in Google's Picker.
 *
 * Picking is also what lets the panel write there: with drive.file, a folder the
 * operator picked is a folder the panel may use, and one they did not is not. So
 * this is the only way to choose one — there is no field to paste an id into.
 */
export function FolderField({
	status,
	folder,
	onChange,
}: {
	status: GoogleStatus | null;
	folder: BackupFolder | null;
	onChange: (folder: BackupFolder) => void;
}) {
	const [error, setError] = useState<string | null>(null);
	const [busy, setBusy] = useState(false);
	const ready = Boolean(status?.connected && status.pickerApiKey && status.pickerAppId);

	async function choose() {
		setError(null);
		setBusy(true);
		try {
			const token = await pickerToken();
			if (!token.ok) {
				setError(token.error);
				return;
			}
			const picked = await pickFolder({
				accessToken: token.data.accessToken,
				apiKey: status?.pickerApiKey ?? "",
				appId: status?.pickerAppId ?? "",
			});
			if (picked) onChange(picked);
		} catch (failure) {
			setError(failure instanceof Error ? failure.message : "the folder picker failed to open");
		} finally {
			setBusy(false);
		}
	}

	return (
		<div className="flex flex-col gap-2">
			<Label>Drive folder</Label>
			<div className="flex items-center gap-3">
				<Button type="button" variant="secondary" icon={FolderOpen} disabled={!ready || busy} onClick={choose}>
					{folder ? "Change folder" : "Choose folder"}
				</Button>
				<span className="truncate text-sm text-muted-foreground">{folder?.name ?? "None chosen"}</span>
			</div>
			{!ready ? (
				<p className="text-xs text-muted-foreground">
					{status?.connected
						? "The Picker needs admin.api.backups.google.picker.apiKey and appId set."
						: "Connect Google Drive first."}
				</p>
			) : null}
			<Banner variant="error" message={error} />
		</div>
	);
}
