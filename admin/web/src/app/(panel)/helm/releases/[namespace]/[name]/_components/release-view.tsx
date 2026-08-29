"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { History, SlidersHorizontal, Trash2 } from "lucide-react";
import Link from "next/link";
import { uninstallRelease } from "@/app/actions/helm";
import { Banner, Button, buttonVariants, Card, cn, InlineDeleteConfirm, Th } from "@/components/ui";
import { Directory } from "../../../../../_components/directory";
import { useRowDelete } from "../../../../../_components/use-row-delete";
import { StatusBadge } from "../../../../_components/status-badge";
import { formatAge } from "@/lib/format/age";
import { describeOutcome, isPending, stuckForMinutes } from "@/lib/helm/status";
import type { HelmReleaseDetail, HelmRevision } from "@/lib/api/types";
import { RevisionRow } from "./revision-row";
import { useReleasePolling } from "./use-release-polling";

export function ReleaseView({
	release,
	history,
	historyError,
	deploymentId,
	deploymentsError,
	now,
}: {
	release: HelmReleaseDetail;
	history: HelmRevision[];
	historyError: string | null;
	/**
	 * The deployment this lab declared for this release, when there is one. Null
	 * means either that the release was installed by something else — by hand, by
	 * the installer script, by another tool — or that the lookup failed, which is
	 * what deploymentsError distinguishes.
	 */
	deploymentId: string | null;
	/** Why the deployment lookup failed, when it did. */
	deploymentsError: string | null;
	now: Date;
}) {
	// Two error sources, kept apart on purpose. historyError is a property of the
	// data this render was given, so it must be read every render -- this page
	// polls, and holding it in state would leave a recovered history showing a
	// stale banner and a newly failed one showing none. actionError is what a
	// rollback or an uninstall reported, which no re-render can recompute.
	const [actionError, setActionError] = useState<string | null>(null);
	const rowDelete = useRowDelete(setActionError);
	const router = useRouter();
	const error = actionError ?? historyError ?? deploymentsError;

	// The 202 design makes this page the progress indicator: nothing else reports
	// what an install or upgrade did, because there is no job to poll.
	useReleasePolling(isPending(release.status));

	const outcome = describeOutcome(release);
	const stuck = stuckForMinutes(release, now);

	return (
		<div className="flex flex-col gap-6">
			<Banner variant="error" message={error} />

			{stuck !== null ? (
				<Banner
					variant="warning"
					message={
						`This release has been ${release.status} for ${stuck} minutes. ` +
						"An operation that was interrupted leaves it this way and Helm refuses " +
						"every later attempt until it is resolved — rolling back to a revision below " +
						"is how to clear it."
					}
				/>
			) : null}

			{outcome.state === "failed" ? (
				<Banner variant="error" message={outcome.reason} />
			) : null}

			<Card padding="md">
				<div className="flex flex-wrap items-start justify-between gap-4">
					<dl className="grid gap-x-8 gap-y-2 text-sm sm:grid-cols-2">
						<Fact label="Status" value={<StatusBadge status={release.status} />} />
						<Fact label="Revision" value={String(release.revision)} />
						<Fact label="Chart" value={`${release.chart} ${release.chartVersion}`} />
						<Fact label="App version" value={release.appVersion || "—"} />
						<Fact label="Namespace" value={release.namespace} />
						<Fact label="Updated" value={formatAge(release.updatedAt, now)} />
					</dl>

					<div className="flex items-center gap-2">
						{rowDelete.confirmingId === release.name ? (
							<InlineDeleteConfirm
								label="Uninstall and remove everything it created?"
								confirmLabel="Uninstall"
								busy={rowDelete.isDeleting(release.name)}
								// Leave for the list on success. Uninstalling revalidates this
								// route, which then re-reads a release that is on its way out
								// — so staying here means an error banner about a read that
								// failed for the reason the operator just asked for. The
								// uninstall worked; the list is the page that can still say
								// something true.
								onConfirm={() =>
									rowDelete.confirm(release.name, async () => {
										const result = await uninstallRelease(release.namespace, release.name);
										if (result.ok) router.replace("/helm/releases");
										return result;
									})
								}
								onCancel={rowDelete.cancel}
							/>
						) : (
							<>
								{/* Changing a release means changing the values that produced
								    it, and those live on its deployment page. A second editor
								    here would be a second place to declare the same thing. */}
								{deploymentId ? (
									// A link, wearing the primary button's classes rather than
									// being one: this navigates, and a button that navigates
									// loses middle-click, open-in-new-tab, and the status bar.
									<Link
										href={`/helm/deployments/${deploymentId}`}
										className={cn(buttonVariants.primary, "flex items-center gap-2")}
									>
										<SlidersHorizontal className="h-4 w-4" />
										Values and rollout
									</Link>
								) : deploymentsError ? (
									// Not "installed outside the panel": that is a claim, and what
									// happened is that the question could not be asked.
									<span className="text-xs text-muted-foreground">
										Could not tell whether this release was declared here
									</span>
								) : (
									<span className="text-xs text-muted-foreground">
										Installed outside the panel &mdash; readable here, editable once declared
									</span>
								)}
								<Button
									variant="secondary"
									icon={Trash2}
									onClick={() => rowDelete.ask(release.name)}
								>
									Uninstall
								</Button>
							</>
						)}
					</div>
				</div>
			</Card>

			<Directory
				error={null}
				isEmpty={history.length === 0}
				minWidth="min-w-[640px]"
				empty={{ icon: History, title: "No history", description: "This release has one revision." }}
				columns={
					<>
						<Th className="text-right">Revision</Th>
						<Th>Status</Th>
						<Th>Chart</Th>
						<Th>Description</Th>
						<Th className="text-right">Updated</Th>
						<Th className="text-right">Actions</Th>
					</>
				}
				rows={history.map((revision) => (
					<RevisionRow
						key={revision.revision}
						revision={revision}
						release={release}
						now={now}
						onError={setActionError}
					/>
				))}
			/>

		</div>
	);
}

function Fact({ label, value }: { label: string; value: React.ReactNode }) {
	return (
		<div className="flex items-center gap-2">
			<dt className="text-muted-foreground">{label}</dt>
			<dd className="text-foreground">{value}</dd>
		</div>
	);
}
