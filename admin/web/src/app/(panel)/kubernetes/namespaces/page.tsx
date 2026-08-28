import { listNamespaces } from "@/app/actions/kube";
import { NamespaceList } from "./_components/namespace-list";

export const dynamic = "force-dynamic";

/**
 * Fetches on the server and hands the rows to a client component, which owns only
 * what needs a browser: the create panel and the delete confirmations.
 *
 * Every namespace is listed, protected ones included. Protection is about
 * writing, and hiding platform-system would take away the reading this panel
 * exists for — so a protected row is shown with its reason and without a delete.
 */
export default async function KubernetesNamespacesPage() {
	const namespaces = await listNamespaces();

	return (
		<NamespaceList
			namespaces={namespaces.ok ? namespaces.data : []}
			// Ages are computed against one instant taken on the server, the same
			// as every other cluster list. Taking it in the client component would
			// give the server and the browser two different answers for the same
			// row and a hydration mismatch to go with them.
			now={new Date()}
			loadError={namespaces.ok ? null : namespaces.error}
		/>
	);
}
