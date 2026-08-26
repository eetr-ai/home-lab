/**
 * Rendering cluster capacity for a person rather than a machine.
 *
 * The API reports CPU in millicores because that is the unit requests are
 * written in, and a dashboard has to show both: "1.5 cores" for a total, and
 * "250m" for what one workload reserved.
 */

/** How many millicores make a core. */
const MILLIS_PER_CORE = 1000;

/**
 * A CPU quantity as text.
 *
 * Under a core it stays in millicores, because "0.1 cores" is how nobody writes
 * a request and "100m" is how everybody does. At or above one core it switches,
 * because "4000m" is a number to be divided rather than read.
 */
export function formatCores(millis: number): string {
	if (!Number.isFinite(millis) || millis < 0) return "—";
	if (millis < MILLIS_PER_CORE) return `${Math.round(millis)}m`;

	const cores = millis / MILLIS_PER_CORE;
	// A whole number of cores is the common case for a node's capacity, and
	// "4.0 cores" reads as a measurement where "4 cores" reads as a fact.
	const text = Number.isInteger(cores) ? String(cores) : cores.toFixed(1);
	return `${text} ${cores === 1 ? "core" : "cores"}`;
}

/**
 * What fraction of `total` is `part`, as a percentage from 0 to 100.
 *
 * Null rather than zero when there is nothing to divide by. A meter with no
 * denominator has no fill to draw, and drawing an empty bar would claim a
 * measurement of zero that was never taken.
 */
export function percentOf(part: number, total: number): number | null {
	if (!Number.isFinite(part) || !Number.isFinite(total) || total <= 0) return null;
	if (part < 0) return null;
	// Above the total is possible and worth showing rather than hiding: a node
	// can be overcommitted, and clamping would make it look merely full.
	return Math.min((part / total) * 100, 100);
}

/** The severity a usage meter should read as. */
export type Level = "normal" | "warning" | "critical";

const WARNING_AT = 75;
const CRITICAL_AT = 90;

/**
 * How alarming a percentage is.
 *
 * The thresholds are here rather than in the component so they are one decision
 * with tests rather than a number repeated per tile.
 */
export function levelFor(percent: number | null): Level {
	if (percent === null) return "normal";
	if (percent >= CRITICAL_AT) return "critical";
	if (percent >= WARNING_AT) return "warning";
	return "normal";
}
