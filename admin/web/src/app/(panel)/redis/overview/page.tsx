import { Activity, Clock, Gauge, KeyRound, MemoryStick, Plug } from "lucide-react";
import { readOverview } from "@/app/actions/redis";
import { Banner } from "@/components/ui/banner";
import { SectionCard } from "@/components/ui/card";
import { formatBytes } from "@/lib/format/bytes";
import { formatUptime, hitRate, memoryTone } from "@/lib/redis/overview";
import { Meter } from "../../_components/meter";
import { Stat } from "../../_components/stat";

export const dynamic = "force-dynamic";

/**
 * What the shared Redis says about itself.
 *
 * Memory leads, because under `noeviction` a full Redis refuses writes rather
 * than forgetting old keys, and the first writer to find out is the panel's own
 * token refresh. Refused writes are counted beside it so "it is fine now" and "it
 * was not fine an hour ago" can both be read off the same tile.
 */
export default async function RedisOverviewPage() {
	const result = await readOverview();
	if (!result.ok) {
		return <Banner variant="error" message={`Redis could not be read: ${result.error}`} />;
	}

	const overview = result.data;
	const tone = memoryTone(overview.usedMemoryBytes, overview.maxMemoryBytes, overview.oomRejections);
	const hits = hitRate(overview.keyspaceHits, overview.keyspaceMisses);
	const keys = overview.keyspace.reduce((sum, db) => sum + db.keys, 0);
	const expiring = overview.keyspace.reduce((sum, db) => sum + db.expires, 0);

	return (
		<div className="flex flex-col gap-6">
			<div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
				<Stat
					icon={MemoryStick}
					label="Memory"
					value={formatBytes(overview.usedMemoryBytes)}
					tone={tone}
					detail={
						overview.oomRejections > 0
							? `${overview.oomRejections} writes refused for memory since it started`
							: overview.maxMemoryBytes > 0
								? `of ${formatBytes(overview.maxMemoryBytes)}, ${overview.maxMemoryPolicy}`
								: "no ceiling"
					}
				/>
				<Stat
					icon={KeyRound}
					label="Keys"
					value={keys.toLocaleString()}
					detail={`${expiring.toLocaleString()} with a TTL`}
				/>
				<Stat
					icon={Plug}
					label="Clients"
					value={overview.connectedClients.toLocaleString()}
					tone={overview.rejectedConnections > 0 ? "warning" : "normal"}
					detail={
						overview.rejectedConnections > 0
							? `${overview.rejectedConnections} connections refused`
							: `${overview.blockedClients} blocked, ${overview.maxClients.toLocaleString()} allowed`
					}
				/>
				<Stat
					icon={Clock}
					label="Uptime"
					value={formatUptime(overview.uptimeSeconds)}
					detail={`Redis ${overview.version}, ${overview.mode}`}
				/>
			</div>

			<SectionCard title="Memory" icon={Gauge}>
				<div className="space-y-4">
					<Meter
						label="In use against the ceiling"
						part={overview.usedMemoryBytes}
						total={overview.maxMemoryBytes}
						format={formatBytes}
					/>
					<Meter
						label="Peak against the ceiling"
						part={overview.peakMemoryBytes}
						total={overview.maxMemoryBytes}
						format={formatBytes}
					/>
				</div>
			</SectionCard>

			<SectionCard title="Traffic since start" icon={Activity}>
				<dl className="grid grid-cols-2 gap-4 text-sm sm:grid-cols-4">
					<Figure label="Commands" value={overview.commandsProcessed.toLocaleString()} />
					<Figure label="Hit rate" value={hits === null ? "—" : `${hits.toFixed(1)}%`} />
					<Figure label="Expired keys" value={overview.expiredKeys.toLocaleString()} />
					<Figure label="Evicted keys" value={overview.evictedKeys.toLocaleString()} />
				</dl>
			</SectionCard>
		</div>
	);
}

function Figure({ label, value }: { label: string; value: string }) {
	return (
		<div>
			<dt className="text-muted-foreground">{label}</dt>
			<dd className="mt-1 font-mono">{value}</dd>
		</div>
	);
}
