import { levelFor, percentOf } from "@/lib/format/resources";

/**
 * A proportion, drawn as a bar.
 *
 * A null percentage draws no bar at all rather than an empty one. "Nothing
 * measured this" and "this is at zero" are different claims, and a track with no
 * fill is how the second one looks.
 */
export function Meter({
	label,
	part,
	total,
	format,
}: {
	label: string;
	part: number;
	total: number;
	/** Renders both numbers, so the caller decides bytes versus cores. */
	format: (value: number) => string;
}) {
	const percent = percentOf(part, total);
	const level = levelFor(percent);

	return (
		<div>
			<div className="flex items-baseline justify-between gap-2 text-sm">
				<span className="truncate text-muted-foreground">{label}</span>
				<span className="shrink-0 font-mono text-xs">
					{percent === null ? "—" : `${format(part)} / ${format(total)}`}
				</span>
			</div>
			<div
				className="mt-1.5 h-2 overflow-hidden rounded-chip bg-surface-sunken"
				role="progressbar"
				aria-label={label}
				aria-valuenow={percent === null ? undefined : Math.round(percent)}
				aria-valuemin={0}
				aria-valuemax={100}
			>
				{percent === null ? null : (
					<div className={`h-full ${FILLS[level]}`} style={{ width: `${percent}%` }} />
				)}
			</div>
		</div>
	);
}

// The theme has no dedicated fill roles, so a meter borrows the icon colours —
// they are the tokens that carry the same "this is fine / this is not" meaning,
// and they flip with the theme like everything else. Raw ramps would fail
// scripts/check-theme.mjs, which is the point of having roles at all.
const FILLS = {
	normal: "bg-brand",
	warning: "bg-warning-fg",
	critical: "bg-danger-icon",
} as const;
