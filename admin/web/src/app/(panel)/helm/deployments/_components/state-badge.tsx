import { cn } from "@/components/ui/cn";
import type { HelmDeploymentState } from "@/lib/api/types";

/**
 * How a declared deployment stands against the cluster.
 *
 * Each state gets a sentence rather than only a colour, because the words are
 * the useful part: "drifted" is actionable and a yellow dot is not. The titles
 * are what an operator hovers when they want to know what the panel means.
 */
const states: Record<HelmDeploymentState, { label: string; title: string; tone: string }> = {
	"in-sync": {
		label: "In sync",
		title: "The cluster is running the newest declared version.",
		tone: "border-success-border bg-success-bg text-success-fg",
	},
	pending: {
		label: "Not rolled out",
		title: "The newest declared version has never been applied. Roll it out to deploy it.",
		tone: "border-accent-fg/30 bg-accent-bg text-accent-fg",
	},
	drifted: {
		label: "Drifted",
		title:
			"The cluster is running a different chart version from the newest one rolled out " +
			"from here — something changed the release outside the panel.",
		tone: "border-warning-border bg-warning-bg text-warning-fg",
	},
	"not-installed": {
		label: "Not installed",
		title: "This lab has a record and Helm has no release. Roll it out to install it.",
		tone: "border-border-strong bg-surface-sunken text-muted-foreground",
	},
	unknown: {
		label: "Unknown",
		title:
			"The live release could not be read, so nothing can be said about whether the " +
			"record matches it.",
		tone: "border-danger-border bg-danger-bg text-danger-fg",
	},
};

export function StateBadge({ state }: { state: HelmDeploymentState }) {
	const described = states[state] ?? states.unknown;
	return (
		<span
			title={described.title}
			className={cn(
				"inline-flex items-center whitespace-nowrap rounded-chip border px-2 py-0.5 text-xs font-medium",
				described.tone,
			)}
		>
			{described.label}
		</span>
	);
}
