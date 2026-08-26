import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { Card } from "@/components/ui/card";

/**
 * One figure on the dashboard.
 *
 * Deliberately not a Client Component: the overview is a Server Component and
 * passes `icon`, which is a function. See the note in _components/directory.tsx —
 * marking this "use client" fails at render time rather than at build time.
 */
export function Stat({
	icon: Icon,
	label,
	value,
	detail,
	tone = "normal",
}: {
	icon: LucideIcon;
	label: string;
	value: ReactNode;
	/** The second line: a denominator, a breakdown, or why there is no value. */
	detail?: ReactNode;
	/** Colours the value, for a figure that is itself the bad news. */
	tone?: "normal" | "warning" | "danger";
}) {
	return (
		<Card padding="sm">
			<div className="flex items-center gap-2 text-sm text-muted-foreground">
				<Icon className="h-4 w-4 shrink-0" />
				<span className="truncate">{label}</span>
			</div>
			<p className={`mt-2 text-2xl font-semibold ${TONES[tone]}`}>{value}</p>
			{detail ? <p className="mt-1 text-xs text-muted-foreground">{detail}</p> : null}
		</Card>
	);
}

const TONES = {
	normal: "",
	warning: "text-warning-fg",
	danger: "text-danger-fg",
} as const;
