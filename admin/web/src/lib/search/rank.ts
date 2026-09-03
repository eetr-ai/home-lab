/**
 * Subsequence ranking for the searchable pickers.
 *
 * Ported from the octo project's `@octo/util` search: a single-pass subsequence
 * scorer, not edit distance — the query's letters must appear in the target in
 * order (not necessarily adjacent), and the score falls off with how spread out
 * they are. A prefix scores 1, an unrelated string 0. It is pure and has no
 * imports, so it lives in `lib` where it can be tested directly.
 */

export type RankBias = "unbiased" | "favor-closer-length";

/**
 * Score `query` against `target`, case-insensitively, from 0 (nothing matched)
 * to 1 (every letter, tightly packed).
 */
export function rankSearchString(
	query: string,
	target: string,
	bias: RankBias = "unbiased",
): number {
	if (typeof query !== "string" || typeof target !== "string") return 0;

	const q = query.toLowerCase();
	const haystack = target.toLowerCase();

	let best = packed(q, haystack, 0);
	if (best < 1 && q.length > 0) {
		// Re-run from every place the first letter occurs, to escape a greedy local
		// minimum — "ab" against "aab" would otherwise miss the tighter second span.
		for (let at = haystack.indexOf(q[0]); at !== -1; at = haystack.indexOf(q[0], at + 1)) {
			best = Math.max(best, packed(q, haystack, at));
			if (best === 1) break;
		}
	}

	if (bias === "unbiased" || best === 0) return best;

	// Mix in how close the lengths are, so "ord" ranks "orders" above
	// "orders-reconciliation-worker".
	const ratio = q.length > target.length ? target.length / q.length : q.length / target.length;
	return 0.3 * ratio + 0.7 * best;
}

/**
 * How tightly `q`'s letters sit in `haystack`, looking no earlier than `from`.
 * Greedy: each letter takes the next place it appears.
 */
function packed(q: string, haystack: string, from: number): number {
	let firstIndex = -1;
	let lastIndex = -1;
	let lastMatchedInQuery = 0;
	let found = 0;

	for (let i = 0; i < q.length; i++) {
		const at = haystack.indexOf(q[i], lastIndex >= 0 ? lastIndex + 1 : from);
		if (at === -1) continue;

		lastIndex = at;
		found++;
		if (firstIndex === -1) {
			firstIndex = at;
			continue;
		}
		lastMatchedInQuery = i;
	}

	const trailing = q.length - lastMatchedInQuery;
	const missed = q.length - found;
	const span = Math.max(lastIndex + trailing - firstIndex, q.length) + missed;
	if (span <= 0) return 0;

	return found / span;
}

/** Options for {@link filterRanked}. */
export interface FilterRankedOptions {
	/** Lowest score still worth showing. Defaults to 0.4 — a weak but real match. */
	min?: number;
	/** Keep at most this many results. Unlimited by default. */
	limit?: number;
	bias?: RankBias;
}

/**
 * Rank `items` against `query` and return the ones worth showing, best first.
 * An empty query returns everything, in original order.
 */
export function filterRanked<T>(
	items: readonly T[],
	query: string,
	toText: (item: T) => string,
	options: FilterRankedOptions = {},
): T[] {
	const { min = 0.4, limit, bias = "favor-closer-length" } = options;

	const trimmed = query.trim();
	if (!trimmed) return limit === undefined ? [...items] : items.slice(0, limit);

	const scored: { item: T; score: number; at: number }[] = [];
	items.forEach((item, at) => {
		const score = rankSearchString(trimmed, toText(item), bias);
		if (score >= min) scored.push({ item, score, at });
	});

	// Best first, and among equals the caller's own order — a stable tie so a
	// re-render never reshuffles matches that scored the same.
	scored.sort((a, b) => b.score - a.score || a.at - b.at);

	const ranked = scored.map((entry) => entry.item);
	return limit === undefined ? ranked : ranked.slice(0, limit);
}
