import { ShipWheel } from "lucide-react";
import { listReleases } from "@/app/actions/helm";
import { Th } from "@/components/ui/table";
import { Directory } from "../../_components/directory";
import { ReleaseRows } from "./_components/release-rows";

export const dynamic = "force-dynamic";

/**
 * Every release in every managed namespace.
 *
 * The fetching stays on the server; only the rows are a client component, because
 * the whole row is clickable and that needs a router. Installing happens from the
 * catalog tab, and everything that can be done to a release is done on the
 * release's own page — a list where each row carries four actions is a list
 * nobody can scan.
 */
export default async function HelmReleasesPage() {
	const releases = await listReleases();
	const now = new Date();

	return (
		<Directory
			error={releases.ok ? null : releases.error}
			isEmpty={releases.ok && releases.data.length === 0}
			minWidth="min-w-[720px]"
			empty={{
				icon: ShipWheel,
				title: "No releases",
				description: "Nothing is installed in the namespaces this lab manages.",
			}}
			columns={
				<>
					<Th>Release</Th>
					<Th>Namespace</Th>
					<Th>Chart</Th>
					<Th>Status</Th>
					<Th className="text-right">Updated</Th>
				</>
			}
			rows={<ReleaseRows releases={releases.ok ? releases.data : []} now={now} />}
		/>
	);
}
