import { Server } from "lucide-react";
import { listNodes } from "@/app/actions/kube";
import { Td, Th } from "@/components/ui/table";
import { Directory } from "../../_components/directory";
import { formatAge } from "@/lib/format/age";
import { formatBytes } from "@/lib/format/bytes";
import { formatCores, percentOf } from "@/lib/format/resources";
import type { Filesystem, Resources } from "@/lib/api/types";

export const dynamic = "force-dynamic";

/**
 * The machines behind the cluster.
 *
 * Not namespace-scoped, unlike every other tab here: a node belongs to the
 * cluster rather than to a namespace, so this page has no ScopePicker.
 */
export default async function NodesPage() {
	const nodes = await listNodes();
	const rows = nodes.ok ? nodes.data : [];
	const now = new Date();

	return (
		<Directory
			error={nodes.ok ? null : nodes.error}
			/* Not just an empty list: a read that failed returns one too, and "the
			   cluster reported no machines" is a claim a failed read cannot make. */
			isEmpty={nodes.ok && rows.length === 0}
			minWidth="min-w-[900px]"
			empty={{
				icon: Server,
				title: "No nodes",
				description: "The cluster reported no machines, which usually means the panel is reading the wrong one.",
			}}
			columns={
				<>
					<Th>Name</Th>
					<Th>Status</Th>
					<Th>Roles</Th>
					<Th className="text-right">CPU</Th>
					<Th className="text-right">Memory</Th>
					<Th className="text-right">Disk</Th>
					<Th>Version</Th>
					<Th className="text-right">Age</Th>
				</>
			}
			rows={rows.map((node) => (
				<tr key={node.name}>
					<Td className="font-medium">{node.name}</Td>
					<Td className={node.ready ? "text-muted-foreground" : "text-danger-fg"}>
						{node.status}
						{node.pressure.length > 0 ? (
							<span className="block text-xs text-warning-fg">{node.pressure.join(", ")}</span>
						) : null}
					</Td>
					<Td className="text-muted-foreground">{node.roles.join(", ") || "—"}</Td>
					<Td className="text-right text-muted-foreground">
						<Load
							used={node.usage?.cpuMillis}
							requested={node.requested.cpuMillis}
							allocatable={node.allocatable.cpuMillis}
							format={formatCores}
						/>
					</Td>
					<Td className="text-right text-muted-foreground">
						<Load
							used={node.usage?.memoryBytes}
							requested={node.requested.memoryBytes}
							allocatable={node.allocatable.memoryBytes}
							format={formatBytes}
						/>
					</Td>
					<Td className="text-right text-muted-foreground">
						<Disk filesystem={node.filesystem} allocatable={node.allocatable} />
					</Td>
					<Td className="font-mono text-xs text-muted-foreground">{node.version}</Td>
					<Td className="text-right text-muted-foreground">{formatAge(node.createdAt, now)}</Td>
				</tr>
			))}
		/>
	);
}

/**
 * One resource on one node: what is being used, over what can be scheduled.
 *
 * Reservations are always shown and usage only when something measured it. The
 * two answer different questions — whether more can be scheduled here, and
 * whether what was already reserved is being used — and a node with no metrics
 * still answers the first.
 */
function Load({
	used,
	requested,
	allocatable,
	format,
}: {
	used: number | undefined;
	requested: number;
	allocatable: number;
	format: (value: number) => string;
}) {
	const share = percentOf(requested, allocatable);
	return (
		<>
			<span className="font-mono text-xs">
				{used === undefined ? "—" : format(used)} used
			</span>
			<span className="block font-mono text-xs">
				{format(requested)} / {format(allocatable)}
				{share === null ? "" : ` (${Math.round(share)}%)`}
			</span>
		</>
	);
}

/**
 * The node's root disk, when the panel is permitted to read it.
 *
 * Falls back to the ephemeral-storage allocatable, which is a capacity and not a
 * usage — so it is labelled as such rather than shown as a figure that looks like
 * the real thing. See admin.api.kubernetes.nodeStats in the chart.
 */
function Disk({
	filesystem,
	allocatable,
}: {
	filesystem: Filesystem | undefined;
	allocatable: Resources;
}) {
	if (!filesystem) {
		// Allocatable, not free. This is capacity minus what the kubelet and the OS
		// have reserved — the ceiling the scheduler measures pod requests against,
		// and it does not move as pods fill the disk. Labelling it "free" would
		// read as a usage figure, which is the one thing it is not.
		const allocatableDisk = allocatable.ephemeralBytes ?? 0;
		return (
			<span className="font-mono text-xs">
				{allocatableDisk > 0 ? `${formatBytes(allocatableDisk)} allocatable to pods` : "—"}
			</span>
		);
	}

	const share = percentOf(filesystem.usedBytes, filesystem.capacityBytes);
	return (
		<>
			<span className="font-mono text-xs">{formatBytes(filesystem.usedBytes)} used</span>
			<span className="block font-mono text-xs">
				{formatBytes(filesystem.capacityBytes)}
				{share === null ? "" : ` (${Math.round(share)}%)`}
			</span>
		</>
	);
}
