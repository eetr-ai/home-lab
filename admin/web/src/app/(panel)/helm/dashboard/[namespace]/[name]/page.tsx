import { Banner } from "@/components/ui/banner";
import { listDeployments, listHelmJobs, readHistory, readRelease } from "@/app/actions/helm";
import { ReleaseView } from "./_components/release-view";

export const dynamic = "force-dynamic";

/**
 * One release: what it is, what it was configured with, and what it was before.
 *
 * This is the cluster's view, not this lab's record — everything here comes out
 * of Helm's own storage. A release the panel declared is better looked at from
 * its deployment page, which has the values editor; the link across is what the
 * `deploymentId` is for.
 *
 * Four round trips, made in parallel rather than in sequence. Each failure is
 * reported as itself: a read that fails is not the same as a release that is
 * absent, and collapsing either into the other states something false with more
 * confidence than a bare failure would.
 *
 * The fourth is the job operating on this release, if one is. Looked up rather
 * than remembered, so this page shows a rollback somebody else started as readily
 * as one started here.
 */
export default async function HelmReleasePage({
	params,
}: {
	params: Promise<{ namespace: string; name: string }>;
}) {
	const { namespace, name } = await params;
	const [release, history, deployments, jobs] = await Promise.all([
		readRelease(namespace, name),
		readHistory(namespace, name),
		listDeployments(namespace),
		listHelmJobs({ namespace, release: name }),
	]);

	// Not notFound(): the result carries a message and not a status, so a
	// transport failure and an absent release are indistinguishable here. A 404
	// page would assert the second whenever the first happened. This follows the
	// workload detail page, which does the same for the same reason.
	if (!release.ok) {
		return <Banner variant="error" message={release.error} />;
	}

	// Whether this lab declared this release. Three answers, not two: a listing
	// that failed is not evidence that a release was installed outside the panel,
	// and saying so would be the same error as rendering any other failed read as
	// a fact. Only a successful listing that found nothing means unmanaged.
	const declared = deployments.ok
		? deployments.data.find((deployment) => deployment.releaseName === release.data.name)
		: undefined;

	return (
		<ReleaseView
			backHref={`/helm/dashboard?namespace=${encodeURIComponent(namespace)}`}
			release={release.data}
			history={history.ok ? history.data : []}
			historyError={history.ok ? null : history.error}
			deploymentId={declared?.id ?? null}
			deploymentsError={deployments.ok ? null : deployments.error}
			job={jobs.ok ? (jobs.data[0] ?? null) : null}
			now={new Date()}
		/>
	);
}
