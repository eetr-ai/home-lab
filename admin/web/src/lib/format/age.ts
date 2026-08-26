/**
 * How long ago something happened, in the compact form `kubectl` uses — "12s",
 * "5m", "3h", "4d". Coarse on purpose: the question a workload list answers is
 * "was this restarted recently", not "when exactly".
 */

const MINUTE = 60;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
const YEAR = 365 * DAY;

/** Format an RFC 3339 timestamp as an age relative to `now`. */
export function formatAge(timestamp: string, now: Date): string {
	const then = Date.parse(timestamp);
	if (Number.isNaN(then)) return "—";

	const seconds = Math.floor((now.getTime() - then) / 1000);
	// A timestamp in the future is a clock disagreeing with another clock, not an
	// age. Showing "0s" says so without inventing a negative one.
	if (seconds <= 0) return "0s";

	if (seconds < MINUTE) return `${seconds}s`;
	if (seconds < HOUR) return `${Math.floor(seconds / MINUTE)}m`;
	if (seconds < DAY) return `${Math.floor(seconds / HOUR)}h`;
	if (seconds < YEAR) return `${Math.floor(seconds / DAY)}d`;
	return `${Math.floor(seconds / YEAR)}y`;
}
