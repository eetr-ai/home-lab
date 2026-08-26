import { Activity, Boxes, Container, Cpu, HardDrive, Layers, MemoryStick, Server } from "lucide-react";
import { SectionCard } from "@/components/ui/card";
import { formatBytes } from "@/lib/format/bytes";
import { formatCores } from "@/lib/format/resources";
import type { ClusterSummary, PodSummary } from "@/lib/api/types";
import { plural } from "./plural";
import { Meter } from "./meter";
import { Stat } from "./stat";

/** Why the running count is short, when it is. */
function podDetail(pods: PodSummary): string {
	const notes: string[] = [];
	if (pods.pending > 0) notes.push(`${pods.pending} pending`);
	if (pods.failed > 0) notes.push(`${pods.failed} failed`);
	return notes.length > 0 ? notes.join(", ") : "none pending or failed";
}

/** The headline counts: what exists, and how much of it is healthy. */
function Counts({ summary }: { summary: ClusterSummary }) {
	const { nodes, pods, workloads, storage } = summary;
	return (
		<div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
			<Stat
				icon={Server}
				label="Nodes ready"
				value={`${nodes.ready}/${nodes.total}`}
				detail={nodes.pressure > 0 ? `${plural(nodes.pressure, "node")} under pressure` : "no pressure reported"}
				tone={nodes.ready < nodes.total || nodes.pressure > 0 ? "danger" : "normal"}
			/>
			<Stat
				icon={Boxes}
				label="Workloads"
				value={workloads.total}
				detail={
					workloads.degraded > 0
						? `${plural(workloads.degraded, "workload")} short of replicas`
						: "all at desired replicas"
				}
				tone={workloads.degraded > 0 ? "warning" : "normal"}
			/>
			<Stat
				icon={Container}
				label="Pods running"
				value={`${pods.running}/${pods.total}`}
				detail={podDetail(pods)}
				tone={pods.failed > 0 ? "danger" : "normal"}
			/>
			<Stat
				icon={Layers}
				label="Namespaces"
				value={summary.namespaces}
				detail={plural(storage.claims, "volume claim")}
			/>
		</div>
	);
}

/**
 * Reserved against allocatable, and — when anything measured it — in use against
 * the same denominator.
 *
 * Reservations come first because they are readable with no metrics-server at
 * all, and because they are what decides whether the next workload schedules.
 * Usage answers a different question: whether what was reserved is being used.
 */
function Capacity({ summary }: { summary: ClusterSummary }) {
	const { nodes } = summary;
	return (
		<div className="grid gap-6 lg:grid-cols-2">
			<SectionCard title="Reserved" icon={Activity}>
				<div className="space-y-4">
					<Meter
						label="CPU"
						part={nodes.requested.cpuMillis}
						total={nodes.allocatable.cpuMillis}
						format={formatCores}
					/>
					<Meter
						label="Memory"
						part={nodes.requested.memoryBytes}
						total={nodes.allocatable.memoryBytes}
						format={formatBytes}
					/>
				</div>
			</SectionCard>

			<SectionCard title="In use" icon={Cpu}>
				{summary.metricsAvailable ? (
					<div className="space-y-4">
						<Meter
							label="CPU"
							part={nodes.usage.cpuMillis}
							total={nodes.allocatable.cpuMillis}
							format={formatCores}
						/>
						<Meter
							label="Memory"
							part={nodes.usage.memoryBytes}
							total={nodes.allocatable.memoryBytes}
							format={formatBytes}
						/>
					</div>
				) : (
					/* Not an error. A cluster without metrics-server is a normal cluster,
					   and reporting zero here would be a measurement nobody took. */
					<p className="text-sm text-muted-foreground">
						Live usage is unavailable — metrics-server is not installed, or has not
						collected a sample yet. The reservations beside this are unaffected.
					</p>
				)}
			</SectionCard>
		</div>
	);
}

/** The totals worth a number rather than a bar. */
function Totals({ summary }: { summary: ClusterSummary }) {
	const { nodes, pods, storage } = summary;
	return (
		<div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
			<Stat
				icon={HardDrive}
				label="Volume storage"
				value={formatBytes(storage.capacityBytes)}
				detail={
					storage.unbound > 0
						? `${plural(storage.unbound, "claim")} not bound`
						: `across ${plural(storage.claims, "bound claim")}`
				}
				tone={storage.unbound > 0 ? "warning" : "normal"}
			/>
			<Stat
				icon={Cpu}
				label="CPU allocatable"
				value={formatCores(nodes.allocatable.cpuMillis)}
				detail={`${formatCores(nodes.requested.cpuMillis)} reserved`}
			/>
			<Stat
				icon={MemoryStick}
				label="Memory allocatable"
				value={formatBytes(nodes.allocatable.memoryBytes)}
				detail={`${formatBytes(nodes.requested.memoryBytes)} reserved`}
			/>
			<Stat
				icon={Activity}
				label="Pod restarts"
				value={pods.restarts}
				detail="total, since each pod last started"
				tone={pods.restarts > 0 ? "warning" : "normal"}
			/>
		</div>
	);
}

/** Everything the cluster contributes to the dashboard. */
export function ClusterSection({ summary }: { summary: ClusterSummary }) {
	return (
		<>
			<Counts summary={summary} />
			<Capacity summary={summary} />
			<Totals summary={summary} />
		</>
	);
}
