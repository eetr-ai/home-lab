"use client";

import { Check, Minus } from "lucide-react";
import { Td } from "@/components/ui";
import { InteractiveRow } from "../../../../_components/interactive-row";
import { formatAge } from "@/lib/format/age";
import type { HelmDeploymentVersion } from "@/lib/api/types";

/**
 * One declared version. Clicking it loads its values into the editor above.
 *
 * There is no delete: the history is append-only, and being able to remove a row
 * from it would defeat the one question it exists to answer.
 */
export function VersionRow({
	version,
	selected,
	now,
	onOpen,
}: {
	version: HelmDeploymentVersion;
	selected: boolean;
	now: Date;
	onOpen: () => void;
}) {
	return (
		<InteractiveRow onActivate={onOpen} className={selected ? "bg-surface-hover" : undefined}>
			<Td className="w-px whitespace-nowrap text-right font-medium">{version.version}</Td>
			<Td className="text-muted-foreground">{version.chartVersion}</Td>
			<Td className="text-muted-foreground">{version.createdBy}</Td>
			<Td className="text-muted-foreground">
				{version.source === "ci" ? "pipeline" : "panel"}
			</Td>
			<Td className="text-muted-foreground">
				{version.rolledOutAt ? (
					<span className="inline-flex items-center gap-1.5 whitespace-nowrap text-xs">
						<Check className="h-3.5 w-3.5" />
						{formatAge(version.rolledOutAt, now)}
						{version.helmRevision ? ` · revision ${version.helmRevision}` : ""}
					</span>
				) : (
					<span className="inline-flex items-center gap-1.5 whitespace-nowrap text-xs">
						<Minus className="h-3.5 w-3.5" />
						never
					</span>
				)}
			</Td>
			<Td className="w-px whitespace-nowrap text-right text-muted-foreground">
				{formatAge(version.createdAt, now)}
			</Td>
		</InteractiveRow>
	);
}
