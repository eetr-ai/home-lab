import { listCharts } from "@/app/actions/helm";
import { listNamespaces } from "@/app/actions/kube";
import { CatalogList } from "./_components/catalog-list";

export const dynamic = "force-dynamic";

/**
 * What this lab will install.
 *
 * The catalog is configuration rather than a search: a request names an entry
 * from this list, never a URL, which is what keeps installing bounded. So an
 * empty catalog here means nobody has written one, not that nothing matched.
 *
 * Namespaces come along because installing needs a target. Only the ones Helm may
 * write to are offered, and the API refuses the rest — offering a choice that
 * would be refused is worse than not offering it.
 */
export default async function HelmCatalogPage() {
	const [charts, namespaces] = await Promise.all([listCharts(), listNamespaces()]);

	return (
		<CatalogList
			charts={charts.ok ? charts.data : []}
			namespaces={
				namespaces.ok
					? namespaces.data
							.filter((namespace) => !namespace.protected)
							.map((namespace) => namespace.name)
					: []
			}
			loadError={charts.ok ? null : charts.error}
		/>
	);
}
