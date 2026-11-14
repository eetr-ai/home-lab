/**
 * The key browser's address.
 *
 * Every search pushes a fresh `t`, even for the pattern already on screen, and
 * the browser is keyed by pattern and `t` together. That is what makes "search
 * again" start over — a scan that failed can be retried, and a stale first page
 * replaced — while a revalidation, which re-renders the same address, leaves the
 * operator's place alone.
 *
 * The pattern is sent exactly as typed. Redis key names may begin or end with a
 * space, and trimming ` orders:*` would search a different prefix without
 * saying so. Only an empty pattern is left out.
 */
export function searchHref(pattern: string, now: number): string {
	const query = new URLSearchParams();
	if (pattern !== "") query.set("pattern", pattern);
	query.set("t", String(now));
	return `?${query}`;
}

/** The identity of one search, for keying the browser. */
export function searchKey(pattern: string, t: string | undefined): string {
	return `${pattern}\u0000${t ?? ""}`;
}
