import { Container } from "lucide-react";
import { listPods } from "@/app/actions/kube";
import { Td, Th } from "@/components/ui/table";
import { Directory } from "../../_components/directory";
import { ScopePicker } from "../../_components/scope-picker";
import { resolveNamespace } from "../_components/namespace-scope";
import { formatAge } from "@/lib/format/age";

export const dynamic = "force-dynamic";

/**
 * The API summarizes a pod's state into one word the way `kubectl` does —
 * "Running", "CrashLoopBackOff", "Init:1/2", "Terminating" — because the raw
 * phase says "Running" for a pod whose only container is crash-looping.
 */
const UNHEALTHY = /BackOff|Error|Failed|Unknown|Evicted|OOMKilled/;

export default async function PodsPage({
	searchParams,
}: {
	searchParams: Promise<{ namespace?: string }>;
}) {
	const { namespace: requested } = await searchParams;
	const { namespaces, selected, error } = await resolveNamespace(requested);
	const pods = selected ? await listPods(selected) : null;
	const rows = pods?.ok ? pods.data : [];
	const now = new Date();

	return (
		<>
			<ScopePicker label="Namespace" param="namespace" options={namespaces} selected={selected} />
			<Directory
				error={error ?? (pods && !pods.ok ? pods.error : null)}
				isEmpty={rows.length === 0}
				minWidth="min-w-[760px]"
				empty={{ icon: Container, title: "No pods", description: "Nothing is scheduled in this namespace." }}
				columns={
					<>
						<Th>Name</Th>
						<Th>Status</Th>
						<Th className="text-right">Ready</Th>
						<Th className="text-right">Restarts</Th>
						<Th>Node</Th>
						<Th className="text-right">Age</Th>
					</>
				}
				rows={rows.map((pod) => (
					<tr key={pod.name}>
						<Td className="font-medium">{pod.name}</Td>
						<Td className={UNHEALTHY.test(pod.status) ? "text-danger-fg" : "text-muted-foreground"}>
							{pod.status}
						</Td>
						<Td className="text-right text-muted-foreground">{pod.ready}</Td>
						{/* Restarts are only worth noticing above zero, and then they are
						    worth noticing a lot. */}
						<Td
							className={`text-right ${pod.restarts > 0 ? "text-warning-fg" : "text-muted-foreground"}`}
						>
							{pod.restarts}
						</Td>
						<Td className="text-muted-foreground">{pod.node || "—"}</Td>
						<Td className="text-right text-muted-foreground">{formatAge(pod.createdAt, now)}</Td>
					</tr>
				))}
			/>
		</>
	);
}
