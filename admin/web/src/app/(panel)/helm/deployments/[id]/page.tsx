import { Banner } from "@/components/ui/banner";
import { listHelmJobs, readDeployment } from "@/app/actions/helm";
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
 *
 * The running job is looked up here rather than remembered from whoever started
 * it. That is what makes this page show an operation a colleague began, and — more
 * often — the one that was still running while this panel's own pods were being
 * replaced by it.
 */
export default async function HelmDeploymentPage({
	params,
}: {
	params: Promise<{ id: string }>;
}) {
	const { id } = await params;
	const [deployment, jobs] = await Promise.all([
		readDeployment(id),
		listHelmJobs({ deployment: id }),
	]);

	if (!deployment.ok) {
		return <Banner variant="error" message={deployment.error} />;
	}

	// Newest first from the API, so the first is the one to follow. A failed
	// listing is not worth a banner: it costs the progress panel and nothing else,
	// and the page it would replace is the one that says what is deployed.
	const job = jobs.ok ? (jobs.data[0] ?? null) : null;

	return <DeploymentView deployment={deployment.data} job={job} now={new Date()} />;
}
