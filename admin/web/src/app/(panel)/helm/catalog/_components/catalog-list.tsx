"use client";

import { useState } from "react";
import { AlertTriangle, Package, Plus } from "lucide-react";
import { IconButton, Td, Th } from "@/components/ui";
import { Directory } from "../../../_components/directory";
import type { HelmChartListing } from "@/lib/api/types";
import { InstallPanel } from "./install-panel";

export function CatalogList({
	charts,
	namespaces,
	loadError,
}: {
	charts: HelmChartListing[];
	namespaces: string[];
	loadError: string | null;
}) {
	const [installing, setInstalling] = useState<HelmChartListing | null>(null);

	return (
		<>
			<Directory
				error={loadError}
				isEmpty={charts.length === 0}
				minWidth="min-w-[720px]"
				empty={{
					icon: Package,
					title: "No charts in the catalog",
					description:
						"The catalog is the allowlist of what this lab will install. Add entries to admin.api.helm.charts in the chart's values.",
				}}
				columns={
					<>
						<Th>Chart</Th>
						<Th>Repository</Th>
						<Th>Description</Th>
						<Th>Versions</Th>
						<Th className="text-right">Actions</Th>
					</>
				}
				rows={charts.map((chart) => (
					<tr key={chart.name}>
						<Td className="font-medium">{chart.name}</Td>
						<Td className="text-muted-foreground">{chart.repository}</Td>
						<Td className="text-muted-foreground">{chart.description || "—"}</Td>
						<Td className="text-muted-foreground">
							{chart.unavailable ? (
								/* The repository could not be reached, so this is what
								   configuration alone knows. Saying so matters: a stale
								   list is only a problem when nobody can tell it is stale. */
								<span className="inline-flex items-center gap-1.5 text-xs">
									<AlertTriangle className="h-3.5 w-3.5" />
									{chart.versions.length > 0
										? `${chart.versions.length} pinned — repository unreachable`
										: "repository unreachable"}
								</span>
							) : (
								<span className="text-xs">
									{chart.versions.length} available
									{chart.versions[0] ? ` — latest ${chart.versions[0].version}` : ""}
								</span>
							)}
						</Td>
						<Td className="text-right">
							<IconButton
								aria-label={`Install ${chart.name}`}
								disabled={chart.versions.length === 0 || namespaces.length === 0}
								onClick={() => setInstalling(chart)}
							>
								<Plus className="h-4 w-4" />
							</IconButton>
						</Td>
					</tr>
				))}
			/>

			{/* Keyed by chart, so opening a different one starts from its own
			    versions rather than from whatever was last chosen. */}
			<InstallPanel
				key={installing?.name}
				chart={installing}
				namespaces={namespaces}
				onClose={() => setInstalling(null)}
			/>
		</>
	);
}
