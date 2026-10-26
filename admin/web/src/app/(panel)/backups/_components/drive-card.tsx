"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { CloudOff, HardDrive, Link2, Unlink } from "lucide-react";
import { beginGoogleConnect, disconnectGoogle } from "@/app/actions/backups";
import { Banner, Button, InlineDeleteConfirm, SectionCard } from "@/components/ui";
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
		try {
			const result = await beginGoogleConnect();
			if (!result.ok) {
				setError(result.error);
				setBusy(false);
				return;
			}
			// Stays busy: the page is about to be Google's.
			window.location.assign(result.data.url);
		} catch {
			// An action that never answered, as opposed to one that said no.
			setError("the panel could not reach its server; try again");
			setBusy(false);
		}
	}

	async function disconnect() {
		setError(null);
		setBusy(true);
		try {
			const result = await disconnectGoogle();
			if (!result.ok) setError(result.error);
			else router.replace("/backups");
		} catch {
			setError("the panel could not reach its server; try again");
		} finally {
			setBusy(false);
			setConfirming(false);
		}
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
					{confirming ? (
						// Inline, in the card, per the guidelines' rule for destructive
						// actions; the other buttons step aside so there is one decision.
						<InlineDeleteConfirm
							label="Disconnect? Schedules stay, and fail until an account is connected again."
							confirmLabel="Disconnect"
							busy={busy}
							onConfirm={disconnect}
							onCancel={() => setConfirming(false)}
						/>
					) : (
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
					)}
				</div>
			) : null}

		</SectionCard>
	);
}
