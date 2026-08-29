"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Plus, ShipWheel, Trash2 } from "lucide-react";
import { forgetDeployment } from "@/app/actions/helm";
import { Banner, Button, IconButton, InlineDeleteConfirm, Td, Th } from "@/components/ui";
import { ActionsHeader, Directory } from "../../../_components/directory";
import { InteractiveRow, stopRowActivation } from "../../../_components/interactive-row";
import { useRowDelete } from "../../../_components/use-row-delete";
import { formatAge } from "@/lib/format/age";
import type { HelmDeploymentSummary, Namespace } from "@/lib/api/types";
import { StateBadge } from "./state-badge";
import { DeclarePanel } from "./declare-panel";

export function DeploymentList({
	deployments,
	loadError,
	namespaces,
	namespacesError,
	now,
}: {
	deployments: HelmDeploymentSummary[];
	loadError: string | null;
	namespaces: Namespace[];
	namespacesError: string | null;
	now: Date;
}) {
	const [error, setError] = useState<string | null>(null);
	const [declaring, setDeclaring] = useState(false);
	const rowDelete = useRowDelete(setError);
	const router = useRouter();

	const declare = (
		<Button icon={Plus} onClick={() => setDeclaring(true)} disabled={namespaces.length === 0}>
			New deployment
		</Button>
	);

	return (
		<>
			<div className="mb-4 flex justify-end">{declare}</div>

			{/* Its own banner: the deployments loaded fine, and what failed was the
			    list a new deployment needs a namespace from. Folding it into the
			    page error would say the wrong thing about both. */}
			<Banner variant="warning" message={namespacesError} />

			<Directory
				error={error ?? loadError}
				isEmpty={deployments.length === 0}
				minWidth="min-w-[860px]"
				empty={{
					icon: ShipWheel,
					title: "Nothing declared yet",
					description:
						"Declare a chart by its OCI reference, pick a namespace, and write the values you want.",
					action: declare,
				}}
				columns={
					<>
						<Th>Release</Th>
						<Th>Namespace</Th>
						<Th>Chart</Th>
						<Th className="w-px whitespace-nowrap text-right">Version</Th>
						<Th>State</Th>
						<Th className="w-px whitespace-nowrap text-right">Changed</Th>
						<ActionsHeader />
					</>
				}
				rows={deployments.map((deployment) => (
					<InteractiveRow
						key={deployment.id}
						onActivate={() => router.push(`/helm/deployments/${deployment.id}`)}
					>
						<Td className="font-medium">
							<Link href={`/helm/deployments/${deployment.id}`} onClick={stopRowActivation}>
								{deployment.releaseName}
							</Link>
						</Td>
						<Td className="text-muted-foreground">{deployment.namespace}</Td>
						<Td className="max-w-[22rem] truncate text-muted-foreground" title={deployment.chartRef}>
							{deployment.chartRef}
						</Td>
						<Td className="w-px whitespace-nowrap text-right text-muted-foreground">
							{deployment.current.chartVersion}
						</Td>
						<Td>
							<StateBadge state={deployment.state} />
						</Td>
						<Td className="w-px whitespace-nowrap text-right text-muted-foreground">
							{formatAge(deployment.current.createdAt, now)}
						</Td>
						<Td className="text-right" onClick={stopRowActivation}>
							<Forget deployment={deployment} rowDelete={rowDelete} />
						</Td>
					</InteractiveRow>
				))}
			/>

			<DeclarePanel
				open={declaring}
				namespaces={namespaces}
				onClose={() => setDeclaring(false)}
			/>
		</>
	);
}

/**
 * Forgetting the record, which is not uninstalling the release.
 *
 * The confirm says so in as many words. They are genuinely different operations
 * and the destructive-looking one here is the safer of the two — the workload
 * keeps running either way.
 */
function Forget({
	deployment,
	rowDelete,
}: {
	deployment: HelmDeploymentSummary;
	rowDelete: ReturnType<typeof useRowDelete>;
}) {
	if (rowDelete.confirmingId === deployment.id) {
		return (
			<InlineDeleteConfirm
				label="Forget this deployment? The release stays on the cluster."
				confirmLabel="Forget"
				busy={rowDelete.isDeleting(deployment.id)}
				onConfirm={() => rowDelete.confirm(deployment.id, () => forgetDeployment(deployment.id))}
				onCancel={rowDelete.cancel}
			/>
		);
	}

	return (
		<div className="flex items-center justify-end gap-1">
			<IconButton
				variant="danger"
				aria-label={`Forget ${deployment.releaseName}`}
				onClick={() => rowDelete.ask(deployment.id)}
			>
				<Trash2 className="h-4 w-4" />
			</IconButton>
		</div>
	);
}
