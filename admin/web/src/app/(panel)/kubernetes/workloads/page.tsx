import { Boxes } from "lucide-react";
import { listWorkloads } from "@/app/actions/kube";
import { Th } from "@/components/ui/table";
import { Directory } from "../../_components/directory";
import { ScopePicker } from "../../_components/scope-picker";
import { resolveNamespace } from "../_components/namespace-scope";
import { WorkloadRows } from "./_components/workload-rows";

export const dynamic = "force-dynamic";

/**
 * The fetching stays on the server. The rows are a client component because the
 * whole row is clickable, which needs a router — an underline on one word is not
 * a discoverable way into a detail page.
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
				rows={<WorkloadRows workloads={rows} now={now} />}
			/>
		</>
	);
}
