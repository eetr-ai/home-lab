"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Td } from "@/components/ui";
import { InteractiveRow } from "../../../_components/interactive-row";
import { formatAge } from "@/lib/format/age";
import type { Workload } from "@/lib/api/types";

/**
 * The workload rows, as a client component so the whole row can be clicked.
 *
 * These rows led to a detail page through an underline on one word, which is
 * invisible until the pointer is already on it. The page above still fetches;
 * only the part that needs a router lives here.
 */
export function WorkloadRows({ workloads, now }: { workloads: Workload[]; now: Date }) {
	const router = useRouter();

	return (
		<>
			{workloads.map((workload) => {
				const href =
					`/kubernetes/workloads/${encodeURIComponent(workload.kind)}/${encodeURIComponent(workload.name)}` +
					`?namespace=${encodeURIComponent(workload.namespace)}`;

				return (
					<InteractiveRow
						key={`${workload.kind}/${workload.name}`}
						onActivate={() => router.push(href)}
					>
						<Td className="text-muted-foreground">{workload.kind}</Td>
						<Td className="font-medium">
							<Link
								href={href}
								className="rounded-control outline-none ring-brand group-hover:underline focus-visible:ring-2"
							>
								{workload.name}
							</Link>
						</Td>
						{/* Amber rather than red when short: a rollout in progress is not a
						    fault, and colouring it like one trains you to ignore the colour. */}
						<Td
							className={`text-right ${
								workload.ready < workload.desired ? "text-warning-fg" : "text-muted-foreground"
							}`}
						>
							{workload.ready}/{workload.desired}
						</Td>
						<Td className="truncate font-mono text-xs text-muted-foreground">
							{workload.images.join(", ")}
						</Td>
						<Td className="text-right text-muted-foreground">
							{formatAge(workload.createdAt, now)}
						</Td>
					</InteractiveRow>
				);
			})}
		</>
	);
}
