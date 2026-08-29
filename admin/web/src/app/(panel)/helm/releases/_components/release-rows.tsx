"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Td } from "@/components/ui";
import { InteractiveRow } from "../../../_components/interactive-row";
import { StatusBadge } from "../../_components/status-badge";
import { formatAge } from "@/lib/format/age";
import type { HelmRelease } from "@/lib/api/types";

/**
 * The release rows, as a client component so the whole row can be clicked.
 *
 * The page above stays a Server Component and does the fetching; only the part
 * that needs a router is down here. The name keeps a real Link: that is what a
 * keyboard tabs to and what opens in a new tab on a middle click, and the row
 * click is a convenience layered over it rather than a replacement for it.
 */
export function ReleaseRows({ releases, now }: { releases: HelmRelease[]; now: Date }) {
	const router = useRouter();

	return (
		<>
			{releases.map((release) => {
				const href = `/helm/releases/${encodeURIComponent(release.namespace)}/${encodeURIComponent(release.name)}`;

				return (
					<InteractiveRow
						key={`${release.namespace}/${release.name}`}
						onActivate={() => router.push(href)}
					>
						<Td className="font-medium">
							<Link
								href={href}
								className="rounded-control outline-none ring-brand group-hover:underline focus-visible:ring-2"
							>
								{release.name}
							</Link>
						</Td>
						<Td className="text-muted-foreground">{release.namespace}</Td>
						<Td className="text-muted-foreground">
							{release.chart} {release.chartVersion}
						</Td>
						<Td>
							<StatusBadge status={release.status} />
						</Td>
						<Td className="text-right text-muted-foreground">
							{formatAge(release.updatedAt, now)}
						</Td>
					</InteractiveRow>
				);
			})}
		</>
	);
}
