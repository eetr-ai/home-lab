import { Banner } from "@/components/ui/banner";
import { readDeployment } from "@/app/actions/helm";
import { DeploymentView } from "./_components/deployment-view";

export const dynamic = "force-dynamic";

/**
 * One deployment: its values, its history, and what the cluster is doing with it.
 *
 * A single call, because the API joins the record and the live release itself.
 * Doing that here would mean the panel holding its own opinion about how the two
 * line up, and there would then be two implementations of "drifted" to keep in
 * agreement.
 *
 * Not notFound() on a failed read: the result carries a message and not a status,
 * so a transport failure and an absent deployment are indistinguishable here, and
 * a 404 page would assert the second whenever the first happened.
 */
export default async function HelmDeploymentPage({
	params,
}: {
	params: Promise<{ id: string }>;
}) {
	const { id } = await params;
	const deployment = await readDeployment(id);

	if (!deployment.ok) {
		return <Banner variant="error" message={deployment.error} />;
	}

	return <DeploymentView deployment={deployment.data} now={new Date()} />;
}
