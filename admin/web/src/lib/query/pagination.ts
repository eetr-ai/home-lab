/**
 * The cursor bookkeeping for browsing a table page by page.
 *
 * The server pages forward only: each page carries the cursor for the *next* one,
 * and knows nothing about the previous. So going back is the client's job, and it
 * keeps a breadcrumb — the cursor it used to fetch each page it has visited. Going
 * back is dropping the last crumb and refetching from the one before; the first
 * page's crumb is the empty cursor, which is why the trail always has that floor.
 *
 * Kept here, out of the component, because the one rule worth getting right — you
 * cannot step back past the first page — is a boundary, and boundaries are where
 * off-by-ones live.
 */

/** The trail at the first page: one crumb, the empty cursor. */
export function firstPage(): string[] {
	return [""];
}

/** Move forward onto the page `nextCursor` leads to, remembering how we got there. */
export function advance(trail: string[], nextCursor: string): string[] {
	return [...trail, nextCursor];
}

/** Step back one page. At the first page there is nowhere to go, so it holds. */
export function retreat(trail: string[]): string[] {
	return trail.length > 1 ? trail.slice(0, -1) : trail;
}

/** The cursor that fetches the current page. Empty at the first page. */
export function currentCursor(trail: string[]): string {
	return trail[trail.length - 1] ?? "";
}

/** True once there is a page to step back to. */
export function hasPrevious(trail: string[]): boolean {
	return trail.length > 1;
}

/** The current page number, 1-based, for a "Page N" indicator. */
export function pageNumber(trail: string[]): number {
	return trail.length;
}
