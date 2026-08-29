import { listNamespaceReleases, listReleases } from "@/app/actions/helm";
import { listNamespaces } from "@/app/actions/kube";
import { ScopePicker } from "../../_components/scope-picker";
import { ReleaseTable } from "./_components/release-table";

export const dynamic = "force-dynamic";

/**
 * Everything Helm has, which is not the same as everything this lab declared.
 *
 * The first tab on purpose: it answers "what is actually running", including the
 * releases nobody declared here — the platform chart, the panel itself, anything
 * installed by hand. Deployments answer a different question, and putting this
 * first means the section opens on the cluster rather than on a record of it.
 *
 * With a namespace chosen, the narrower per-namespace read is used rather than
 * filtering a cluster-wide one. Reading a release means reading Secrets, so
 * asking for one namespace instead of all of them is worth the extra branch.
 */
export default async function HelmDashboardPage({
	searchParams,
}: {
	searchParams: Promise<{ namespace?: string }>;
}) {
	const { namespace } = await searchParams;
	const [releases, namespaces] = await Promise.all([
		namespace ? listNamespaceReleases(namespace) : listReleases(),
		listNamespaces(),
	]);

	return (
		<>
			<ScopePicker
				label="Namespace"
				param="namespace"
				allLabel="All namespaces"
				options={namespaces.ok ? namespaces.data.map((namespaceItem) => namespaceItem.name) : []}
				selected={namespace ?? ""}
			/>
			<ReleaseTable
				releases={releases.ok ? releases.data : []}
				loadError={releases.ok ? null : releases.error}
				scoped={Boolean(namespace)}
				now={new Date()}
			/>
		</>
	);
}
