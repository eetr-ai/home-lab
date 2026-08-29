import { jobTone } from "@/lib/helm/status";
import type { HelmJobPhase } from "@/lib/api/types";

/**
 * A Helm job's phase, coloured by what it means.
 *
 * Not a Client Component, so a Server Component page can render it directly —
 * the same reason StatusBadge beside it is not one.
 */
const CLASSES: Record<ReturnType<typeof jobTone>, string> = {
	success: "bg-success-bg text-success-fg",
	danger: "bg-danger-bg text-danger-fg",
	warning: "bg-warning-bg text-warning-fg",
	muted: "bg-surface-sunken text-muted-foreground",
};

export function JobBadge({ phase }: { phase: HelmJobPhase }) {
	return (
		<span
			className={`inline-flex items-center rounded-chip px-2 py-0.5 text-xs font-medium ${CLASSES[jobTone(phase)]}`}
		>
			{phase}
		</span>
	);
}
