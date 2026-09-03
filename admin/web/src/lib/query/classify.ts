/**
 * Which console path a statement takes, decided from its leading keyword.
 *
 * A convenience, not a safety boundary — the difference matters. The console has
 * one Run button, and this picks whether the statement goes to the read endpoint
 * (a READ ONLY transaction the server rolls back) or the write endpoint (a
 * transaction it commits). It does NOT decide what is *allowed*: a misjudged
 * write sent to the read endpoint is refused by PostgreSQL's own read-only
 * transaction, and a misjudged read sent to the write endpoint commits a
 * transaction that changed nothing. So a wrong guess is harmless either way, and
 * this is a leading-keyword check rather than an attempt to parse SQL — which
 * comments, CTEs and dollar quoting all defeat, and which the API deliberately
 * does not do.
 */

/** The statements that only read. Everything else is treated as a write. */
const readKeywords = new Set(["select", "with", "show", "explain", "table", "values"]);

/** A statement mentioning one of these anywhere modifies data — used only to
 *  catch a WITH that wraps a data-modifying CTE. */
const dataModifying = /\b(insert|update|delete|merge)\b/i;

export type StatementKind = "read" | "write";

/** Classify a statement by its first keyword. An unrecognised or empty one reads. */
export function classifyStatement(sql: string): StatementKind {
	const keyword = leadingKeyword(sql);
	if (keyword === null) return "read";
	if (!readKeywords.has(keyword)) return "write";
	// A WITH can wrap a data-modifying CTE — `WITH x AS (DELETE … RETURNING …) …` —
	// which is a write despite the leading keyword. Over-matching a CTE that merely
	// contains the word is harmless (it commits a no-op); under-matching would send a
	// real write to the read-only path, where the transaction refuses it.
	if (keyword === "with" && dataModifying.test(sql)) return "write";
	return "read";
}

/**
 * The first bare word of a statement, lowercased, with leading whitespace and
 * comments skipped — so a query behind a `-- note` still classifies by SELECT.
 */
function leadingKeyword(sql: string): string | null {
	let rest = sql;
	// Strip any run of leading whitespace, line comments, and block comments.
	for (;;) {
		const trimmed = rest.replace(/^\s+/, "");
		const withoutLine = trimmed.replace(/^--[^\n]*/, "");
		const withoutBlock = withoutLine.replace(/^\/\*[\s\S]*?\*\//, "");
		if (withoutBlock === rest) {
			rest = withoutBlock;
			break;
		}
		rest = withoutBlock;
	}
	const match = /^[a-zA-Z]+/.exec(rest);
	return match ? match[0].toLowerCase() : null;
}
