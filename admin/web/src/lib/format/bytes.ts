/**
 * Rendering a byte count for a person rather than a machine.
 *
 * Binary units, because that is what both servers report: PostgreSQL's
 * `pg_database_size` and MongoDB's `dataSize` are both counts of bytes on disk,
 * and a "GB" that meant 10^9 next to a figure meaning 2^30 would be quietly
 * wrong by seven per cent.
 */

const UNITS = ["KiB", "MiB", "GiB", "TiB", "PiB"] as const;

/**
 * A byte count as text, or an em dash when there is no number to show.
 *
 * A size the API could not read comes back as 0, and so does a genuinely empty
 * MongoDB database — the two are told apart by the caller, which knows whether
 * the record claims to be empty, not here.
 */
export function formatBytes(bytes: number): string {
	if (!Number.isFinite(bytes) || bytes < 0) return "—";
	if (bytes < 1024) return `${Math.round(bytes)} B`;

	let value = bytes / 1024;
	let unit = 0;
	// Stop at the last unit rather than running off the end of the array: a home
	// lab will not produce an exabyte, but reading `undefined` if it did would be
	// a worse answer than "1024.0 PiB".
	while (value >= 1024 && unit < UNITS.length - 1) {
		value /= 1024;
		unit += 1;
	}
	return `${value.toFixed(1)} ${UNITS[unit]}`;
}
