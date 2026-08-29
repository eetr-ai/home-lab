import { listDeployments } from "@/app/actions/helm";
import { listNamespaces } from "@/app/actions/kube";
import { DeploymentList } from "./_components/deployment-list";

export const dynamic = "force-dynamic";

/**
 * The charts this lab has declared, and how each stands against the cluster.
 *
 * The namespace list comes along because declaring a deployment needs one to
 * choose from, and protected namespaces are filtered out here rather than in the
 * form: the API refuses them, and offering a choice that would be refused is
 * worse than not offering it. Its failure is reported separately, because "no
 * namespaces" and "the namespaces could not be read" are different sentences.
 */
export default async function HelmDeploymentsPage() {
	const [deployments, namespaces] = await Promise.all([listDeployments(), listNamespaces()]);

	return (
		<DeploymentList
			deployments={deployments.ok ? deployments.data : []}
			loadError={deployments.ok ? null : deployments.error}
			namespaces={namespaces.ok ? namespaces.data.filter((one) => !one.protected) : []}
			namespacesError={namespaces.ok ? null : namespaces.error}
			now={new Date()}
		/>
	);
}
