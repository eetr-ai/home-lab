import { Boxes } from "lucide-react";
import Link from "next/link";
import { listWorkloads } from "@/app/actions/kube";
import { Td, Th } from "@/components/ui/table";
import { Directory } from "../../_components/directory";
import { ScopePicker } from "../../_components/scope-picker";
import { resolveNamespace } from "../_components/namespace-scope";
import { formatAge } from "@/lib/format/age";

export const dynamic = "force-dynamic";

/**
 * A plain Server Component: nothing here is interactive except the namespace
 * picker, so there is no client component to hydrate and no state to hold.
 */
export default async function WorkloadsPage({
	searchParams,
}: {
	searchParams: Promise<{ namespace?: string }>;
}) {
	const { namespace: requested } = await searchParams;
	const { namespaces, selected, error } = await resolveNamespace(requested);
	const workloads = selected ? await listWorkloads(selected) : null;
	const rows = workloads?.ok ? workloads.data : [];
	const now = new Date();

	return (
		<>
			<ScopePicker label="Namespace" param="namespace" options={namespaces} selected={selected} />
			<Directory
				error={error ?? (workloads && !workloads.ok ? workloads.error : null)}
				isEmpty={workloads?.ok === true && rows.length === 0}
				minWidth="min-w-[760px]"
				empty={{ icon: Boxes, title: "Nothing running here", description: "This namespace has no Deployments, StatefulSets, or DaemonSets." }}
				columns={
					<>
						<Th>Kind</Th>
						<Th>Name</Th>
						<Th className="text-right">Ready</Th>
						<Th>Image</Th>
						<Th className="text-right">Age</Th>
					</>
				}
				rows={rows.map((workload) => (
					<tr key={`${workload.kind}/${workload.name}`}>
						<Td className="text-muted-foreground">{workload.kind}</Td>
						<Td className="font-medium">
							<Link
								href={`/kubernetes/workloads/${encodeURIComponent(workload.kind)}/${encodeURIComponent(workload.name)}?namespace=${encodeURIComponent(workload.namespace)}`}
								className="hover:underline"
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
					</tr>
				))}
			/>
		</>
	);
}
