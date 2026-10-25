"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { CloudOff, HardDrive, Link2, Unlink } from "lucide-react";
import { beginGoogleConnect, disconnectGoogle } from "@/app/actions/backups";
import { Banner, Button, ConfirmDialog, SectionCard } from "@/components/ui";
import type { GoogleStatus } from "@/lib/api/types";

/**
 * The Google Drive connection: who is connected, and the way to change it.
 *
 * Connecting leaves the page for Google's consent screen and comes back through
 * the callback route, which is why the outcome arrives as a query parameter
 * rather than as an action result.
 */
export function DriveCard({
	status,
	loadError,
	justConnected,
	connectError,
}: {
	status: GoogleStatus | null;
	loadError: string | null;
	justConnected: boolean;
	connectError: string | null;
}) {
	const [error, setError] = useState<string | null>(null);
	const [busy, setBusy] = useState(false);
	const [confirming, setConfirming] = useState(false);
	const router = useRouter();

	async function connect() {
		setError(null);
		setBusy(true);
		const result = await beginGoogleConnect();
		if (!result.ok) {
			setBusy(false);
			setError(result.error);
			return;
		}
		// Stays busy: the page is about to be Google's.
		window.location.assign(result.data.url);
	}

	async function disconnect() {
		setConfirming(false);
		setBusy(true);
		const result = await disconnectGoogle();
		setBusy(false);
		if (!result.ok) setError(result.error);
		else router.replace("/backups");
	}

	return (
		<SectionCard title="Google Drive" icon={HardDrive}>
			<Banner variant="error" message={error ?? loadError ?? connectError} className="mb-4" />
			<Banner
				variant="success"
				message={justConnected && status?.connected ? "Google Drive is connected." : null}
				className="mb-4"
			/>

			{status && !status.configured ? (
				<p className="flex items-center gap-2 text-sm text-muted-foreground">
					<CloudOff className="h-4 w-4" />
					This lab has no Google OAuth client yet. docs/backups.md walks through creating one.
				</p>
			) : null}

			{status?.configured ? (
				<div className="flex flex-wrap items-center justify-between gap-4">
					<p className="text-sm">
						{status.connected
							? `Backups upload as ${status.account || "the connected account"}.`
							: "No account is connected, so scheduled backups cannot upload."}
						<span className="block text-muted-foreground">
							The panel only sees the files it uploads and the folders you pick for it.
						</span>
					</p>
					<div className="flex gap-2">
						{status.connected ? (
							<Button variant="secondary" icon={Unlink} disabled={busy} onClick={() => setConfirming(true)}>
								Disconnect
							</Button>
						) : null}
						<Button icon={Link2} disabled={busy} onClick={connect}>
							{status.connected ? "Connect another account" : "Connect Google Drive"}
						</Button>
					</div>
				</div>
			) : null}

			<ConfirmDialog
				open={confirming}
				title="Disconnect Google Drive?"
				description="The panel forgets the account and asks Google to revoke its access. Schedules stay, and their runs fail until an account is connected again. Backups already in Drive are not touched."
				confirmLabel="Disconnect"
				onConfirm={disconnect}
				onCancel={() => setConfirming(false)}
			/>
		</SectionCard>
	);
}
