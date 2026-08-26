import { Database, LayoutDashboard, Leaf } from "lucide-react";
import { readSummary } from "@/app/actions/kube";
import { listDatabases as listMongoDatabases } from "@/app/actions/mongo";
import { listDatabases as listPostgresDatabases } from "@/app/actions/postgres";
import { Banner } from "@/components/ui/banner";
import { SectionCard } from "@/components/ui/card";
import { PageHeader } from "@/components/ui/page-header";
import { ClusterSection } from "./_components/cluster-section";
import { DatabaseStat } from "./_components/database-stat";

export const dynamic = "force-dynamic";

/**
 * The panel's landing page: what is running, what it is using, and what is wrong.
 *
 * The rollup across the cluster and both databases happens here rather than in
 * the API. The Go slices do not import each other — see
 * docs/contributing/layer-conventions.md — and a dashboard spanning all three is
 * exactly the change that would tempt one to. The BFF is where a view that
 * crosses slices belongs.
 *
 * Every section reports its own failure rather than one banner standing in for
 * the page. A database that is down must not blank the cluster tiles: the
 * dashboard earns its place precisely when something is broken.
 */
export default async function OverviewPage() {
	const [cluster, mongo, postgres] = await Promise.all([
		readSummary(),
		listMongoDatabases(),
		listPostgresDatabases(),
	]);

	return (
		<main className="flex min-h-screen flex-col gap-6 p-6">
			<PageHeader
				icon={LayoutDashboard}
				title="Overview"
				description="What the lab is running, and what it is using to run it."
			/>

			{cluster.ok ? (
				<ClusterSection summary={cluster.data} />
			) : (
				<Banner variant="error" message={cluster.error} />
			)}

			<SectionCard title="Databases" icon={Database}>
				<div className="grid gap-3 sm:grid-cols-2">
					<DatabaseStat label="PostgreSQL on disk" icon={Database} result={postgres} />
					<DatabaseStat label="MongoDB on disk" icon={Leaf} result={mongo} />
				</div>
			</SectionCard>
		</main>
	);
}
