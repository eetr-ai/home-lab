/**
 * The parts of a query console that are decisions rather than markup.
 *
 * Here rather than in the components so they have tests — vitest runs in node and
 * covers `src/lib` only, deliberately.
 */

/**
 * Parse a MongoDB query document typed into a textarea.
 *
 * A blank field is an empty document, not an error: "show me everything" is the
 * commonest thing to ask, and making it require typing `{}` would be a rule with
 * no reason behind it.
 *
 * Anything that parses to something other than an object is refused rather than
 * sent. `[1,2]` and `"abc"` are both valid JSON, and MongoDB's error for a filter
 * that is an array says less than this can.
 */
export function parseDocument(
	raw: string,
	what: string,
): { document: Record<string, unknown> } | { error: string } {
	const trimmed = raw.trim();
	if (trimmed === "") return { document: {} };

	let parsed: unknown;
	try {
		parsed = JSON.parse(trimmed);
	} catch (err) {
		return { error: `the ${what} is not valid JSON: ${(err as Error).message}` };
	}

	// typeof null is "object", and an array is one too. Both would be accepted by
	// a bare typeof check and refused by the server with a worse message.
	if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
		return { error: `the ${what} must be a JSON object, like {"status": "active"}` };
	}
	return { document: parsed as Record<string, unknown> };
}

/**
 * How a result should be described once it is back.
 *
 * The truncation matters more than the count: a page of 200 rows that is actually
 * the first 200 of 40,000 is a different answer, and presenting it as "200 rows"
 * would let someone read it as the whole table.
 */
export function describeResult(count: number, truncated: boolean, elapsedMs: number): string {
	const rows = `${count} ${count === 1 ? "row" : "rows"}`;
	const timing = `${elapsedMs} ms`;
	return truncated ? `first ${rows} — more were not read — in ${timing}` : `${rows} in ${timing}`;
}
