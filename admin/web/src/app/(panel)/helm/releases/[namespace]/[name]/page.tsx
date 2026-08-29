import { notFound } from "next/navigation";
import { listCharts, readHistory, readRelease } from "@/app/actions/helm";
import { ReleaseView } from "./_components/release-view";

export const dynamic = "force-dynamic";

/**
 * One release: what it is, what it was configured with, and what it was before.
 *
 * The catalog comes along because upgrading offers its versions. Three round
 * trips either way, so they are made in parallel rather than in sequence.
 */
export default async function HelmReleasePage({
	params,
}: {
	params: Promise<{ namespace: string; name: string }>;
}) {
	const { namespace, name } = await params;
	const [release, history, charts] = await Promise.all([
		readRelease(namespace, name),
		readHistory(namespace, name),
		listCharts(),
	]);

	if (!release.ok) notFound();

	// The catalog lists an entry by its key; a release records the chart's own
	// name. Matching on the chart is what decides whether this release can be
	// upgraded from here at all — the API refuses one it did not vet, and offering
	// a version picker that would be refused is worse than offering none.
	const entry = charts.ok
		? charts.data.find((chart) => chart.chart === release.data.chart)
		: undefined;

	return (
		<ReleaseView
			release={release.data}
			history={history.ok ? history.data : []}
			historyError={history.ok ? null : history.error}
			versions={entry?.versions.map((version) => version.version) ?? []}
			inCatalog={entry !== undefined}
			now={new Date()}
		/>
	);
}
