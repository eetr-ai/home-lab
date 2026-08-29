"use client";

import { useState } from "react";
import Link from "next/link";
import { History } from "lucide-react";
import { Banner, Card, Th } from "@/components/ui";
import { Directory } from "../../../../_components/directory";
import { StatusBadge } from "../../../_components/status-badge";
import { StateBadge } from "../../_components/state-badge";
import { isPending, stuckForMinutes } from "@/lib/helm/status";
import { useReleasePolling } from "../../../releases/[namespace]/[name]/_components/use-release-polling";
import type { HelmDeploymentDetail } from "@/lib/api/types";
import { ValuesCard } from "./values-card";
import { VersionRow } from "./version-row";

export function DeploymentView({
	deployment,
	now,
}: {
	deployment: HelmDeploymentDetail;
	now: Date;
}) {
	const [editing, setEditing] = useState(deployment.current.version);

	const release = deployment.release;
	// The 202 design makes this page the progress indicator: nothing else reports
	// what a rollout did, because there is no job to poll.
	useReleasePolling(release ? isPending(release.status) : false);
	const stuck = release ? stuckForMinutes(release, now) : null;

	return (
		<div className="flex flex-col gap-6">
			{/* A failed read of the live release, said out loud. Showing the record
			    with nothing beside it would read as "nothing is deployed", which is
			    a different and much more inviting statement than "I could not look". */}
			<Banner variant="error" message={deployment.releaseError ?? null} />

			{stuck !== null ? (
				<Banner
					variant="warning"
					message={
						`This release has been ${release?.status} for ${stuck} minutes. ` +
						"An operation that was interrupted leaves it this way and Helm refuses every " +
						"later attempt until it is resolved — rolling back from the Releases tab clears it."
					}
				/>
			) : null}

			<Card padding="md">
				<dl className="grid gap-x-8 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-3">
					<Fact label="State" value={<StateBadge state={deployment.state} />} />
					<Fact
						label="Cluster"
						value={release ? <StatusBadge status={release.status} /> : <Muted>no release</Muted>}
					/>
					<Fact
						label="Running"
						value={release ? `${release.chart} ${release.chartVersion}` : <Muted>&mdash;</Muted>}
					/>
					<Fact label="Declared" value={deployment.current.chartVersion} />
					<Fact label="Namespace" value={deployment.namespace} />
					<Fact
						label="Helm revision"
						value={release ? String(release.revision) : <Muted>&mdash;</Muted>}
					/>
					<Fact
						label="Chart"
						value={<span className="break-all text-xs">{deployment.chartRef}</span>}
						wide
					/>
				</dl>

				{release ? (
					<p className="mt-3 text-xs text-muted-foreground">
						<Link
							href={`/helm/releases/${encodeURIComponent(deployment.namespace)}/${encodeURIComponent(deployment.releaseName)}`}
						>
							See the live release, its revisions, and roll back
						</Link>
					</p>
				) : null}
			</Card>

			<ValuesCard deployment={deployment} editing={editing} onEditingChange={setEditing} />

			<Directory
				error={null}
				isEmpty={deployment.versions.length === 0}
				minWidth="min-w-[720px]"
				empty={{ icon: History, title: "No versions", description: "Nothing has been declared yet." }}
				columns={
					<>
						<Th className="w-px whitespace-nowrap text-right">Version</Th>
						<Th>Chart version</Th>
						<Th>Written by</Th>
						<Th>Source</Th>
						<Th>Rolled out</Th>
						<Th className="w-px whitespace-nowrap text-right">When</Th>
					</>
				}
				rows={deployment.versions.map((version) => (
					<VersionRow
						key={version.version}
						version={version}
						selected={version.version === editing}
						now={now}
						onOpen={() => setEditing(version.version)}
					/>
				))}
			/>
		</div>
	);
}

function Fact({
	label,
	value,
	wide = false,
}: {
	label: string;
	value: React.ReactNode;
	wide?: boolean;
}) {
	return (
		<div className={wide ? "flex items-center gap-2 sm:col-span-2 lg:col-span-3" : "flex items-center gap-2"}>
			<dt className="text-muted-foreground">{label}</dt>
			<dd className="text-foreground">{value}</dd>
		</div>
	);
}

function Muted({ children }: { children: React.ReactNode }) {
	return <span className="text-muted-foreground">{children}</span>;
}
