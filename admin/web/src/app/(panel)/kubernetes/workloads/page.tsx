import { Boxes } from "lucide-react";
import { listWorkloads } from "@/app/actions/kube";
import { Th } from "@/components/ui/table";
import { Directory } from "../../_components/directory";
import { NamespacePicker } from "../../_components/namespace-picker";
import { readNamespaceScope } from "../../_components/namespace-scope-server";
import { isAllNamespaces } from "@/lib/kube/scope";
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
	const { namespace } = await searchParams;
	const scope = await readNamespaceScope(namespace);
	const everywhere = isAllNamespaces(scope);
	const workloads = await listWorkloads(scope);
	const rows = workloads.ok ? workloads.data : [];
	const now = new Date();

	return (
		<>
			<NamespacePicker scope={scope} />
			<Directory
				error={workloads.ok ? null : workloads.error}
				isEmpty={workloads.ok && rows.length === 0}
				minWidth={everywhere ? "min-w-[880px]" : "min-w-[760px]"}
				empty={{
					icon: Boxes,
					title: "Nothing running here",
					description: everywhere
						? "The cluster has no Deployments, StatefulSets, or DaemonSets."
						: "This namespace has no Deployments, StatefulSets, or DaemonSets.",
				}}
				columns={
					<>
						{/* Only where the rows come from more than one. A column whose
						    every cell says the same thing is a column that pushed the
						    ones that differ off the edge. */}
						{everywhere ? <Th>Namespace</Th> : null}
						<Th>Kind</Th>
						<Th>Name</Th>
						<Th className="text-right">Ready</Th>
						<Th>Image</Th>
						<Th className="text-right">Age</Th>
					</>
				}
				rows={<WorkloadRows workloads={rows} now={now} showNamespace={everywhere} />}
			/>
		</>
	);
}
