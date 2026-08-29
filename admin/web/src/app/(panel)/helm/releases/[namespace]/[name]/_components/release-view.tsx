"use client";

import { useState } from "react";
import { AlertTriangle, History, Trash2 } from "lucide-react";
import { uninstallRelease } from "@/app/actions/helm";
import { Banner, Button, Card, InlineDeleteConfirm, Th } from "@/components/ui";
import { Directory } from "../../../../../_components/directory";
import { useRowDelete } from "../../../../../_components/use-row-delete";
import { StatusBadge } from "../../../../_components/status-badge";
import { formatAge } from "@/lib/format/age";
import { describeOutcome, isPending, stuckForMinutes } from "@/lib/helm/status";
import type { HelmReleaseDetail, HelmRevision } from "@/lib/api/types";
import { UpgradePanel } from "./upgrade-panel";
import { RevisionRow } from "./revision-row";
import { useReleasePolling } from "./use-release-polling";

export function ReleaseView({
	release,
	history,
	historyError,
	versions,
	inCatalog,
	now,
}: {
	release: HelmReleaseDetail;
	history: HelmRevision[];
	historyError: string | null;
	versions: string[];
	inCatalog: boolean;
	now: Date;
}) {
	const [error, setError] = useState<string | null>(historyError);
	const [upgrading, setUpgrading] = useState(false);
	const rowDelete = useRowDelete(setError);

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
								onConfirm={() =>
									rowDelete.confirm(release.name, () =>
										uninstallRelease(release.namespace, release.name),
									)
								}
								onCancel={rowDelete.cancel}
							/>
						) : (
							<>
								{/* Offered only for a release this lab vetted. The API refuses
								    an upgrade of anything else, and a version picker that
								    would be refused is worse than none. */}
								{inCatalog ? (
									<Button icon={History} onClick={() => setUpgrading(true)}>
										Upgrade
									</Button>
								) : (
									<span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
										<AlertTriangle className="h-3.5 w-3.5" />
										Not in this lab&rsquo;s catalog — upgrade it where it came from
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
						onError={setError}
					/>
				))}
			/>

			<UpgradePanel
				open={upgrading}
				release={release}
				versions={versions}
				onClose={() => setUpgrading(false)}
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
