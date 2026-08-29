import { tone } from "@/lib/helm/status";

/**
 * A Helm status, coloured by what it means.
 *
 * Not a Client Component: the release list is a Server Component and renders
 * this directly. Which tone a status gets is decided in lib/helm/status.ts, where
 * it can be tested without React — the rule that superseded is muted rather than
 * alarming is a judgement, not a formatting detail.
 */
const CLASSES: Record<ReturnType<typeof tone>, string> = {
	success: "bg-success-bg text-success-fg",
	danger: "bg-danger-bg text-danger-fg",
	warning: "bg-warning-bg text-warning-fg",
	muted: "bg-surface-sunken text-muted-foreground",
};

export function StatusBadge({ status }: { status: string }) {
	return (
		<span
			className={`inline-flex items-center rounded-chip px-2 py-0.5 text-xs font-medium ${CLASSES[tone(status)]}`}
		>
			{status}
		</span>
	);
}
