/**
 * Reading the Redis overview for a person.
 *
 * This Redis runs with `noeviction`, which changes what its memory figure means:
 * at the ceiling it does not drop old keys, it refuses writes — and the first
 * writer to notice is the panel's own token refresh. So "how full" is the
 * headline, and the thresholds below are about that rather than about tidiness.
 */

/** Past this share of the ceiling the memory figure is worth a second look. */
const WARN_AT_PERCENT = 75;
/** Past this, writes are close to being refused. */
const DANGER_AT_PERCENT = 90;

export type Tone = "normal" | "warning" | "danger";

/**
 * How worried to be about memory.
 *
 * A server that has already refused writes is in danger whatever it reads now —
 * the figure may have dropped since, but something lost a write, and the count
 * does not reset until Redis restarts. No ceiling at all is normal: there is
 * nothing to approach.
 */
export function memoryTone(used: number, max: number, oomRejections: number): Tone {
	if (oomRejections > 0) return "danger";
	if (max <= 0) return "normal";

	const percent = (used / max) * 100;
	if (percent >= DANGER_AT_PERCENT) return "danger";
	if (percent >= WARN_AT_PERCENT) return "warning";
	return "normal";
}

/**
 * The share of key lookups that found something, or null before there have been
 * any — a hit rate of 0% on a server nobody has asked anything is a claim about
 * a measurement that was never taken.
 */
export function hitRate(hits: number, misses: number): number | null {
	const total = hits + misses;
	if (total <= 0) return null;
	return (hits / total) * 100;
}

/** An uptime in the largest two units that matter. */
export function formatUptime(seconds: number): string {
	if (!Number.isFinite(seconds) || seconds < 0) return "—";
	const days = Math.floor(seconds / 86_400);
	const hours = Math.floor((seconds % 86_400) / 3600);
	const minutes = Math.floor((seconds % 3600) / 60);

	if (days > 0) return `${days}d ${hours}h`;
	if (hours > 0) return `${hours}h ${minutes}m`;
	return `${minutes}m`;
}
