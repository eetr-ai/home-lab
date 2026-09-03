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
	// which is a write despite the leading keyword. Scan the code only, with
	// comments and quoted text removed, so `WITH x AS (SELECT 'delete') …` stays a
	// read: the word in a literal is not a DML statement.
	if (keyword === "with" && dataModifying.test(stripLiteralsAndComments(sql))) return "write";
	return "read";
}

/**
 * Remove comments, string literals, dollar-quoted strings and quoted identifiers,
 * leaving the code between them. Enough to keep the DML scan from tripping over a
 * keyword that only appears inside quotes — not a full SQL parser, which the API
 * deliberately avoids, and which the leading-keyword routing does not need.
 */
export function stripLiteralsAndComments(sql: string): string {
	let out = "";
	let i = 0;
	while (i < sql.length) {
		const pair = sql.slice(i, i + 2);
		if (pair === "--") {
			const newline = sql.indexOf("\n", i);
			i = newline === -1 ? sql.length : newline;
		} else if (pair === "/*") {
			const end = sql.indexOf("*/", i + 2);
			i = end === -1 ? sql.length : end + 2;
		} else if (sql[i] === "'" || sql[i] === '"') {
			i = skipQuoted(sql, i, sql[i]);
		} else if (sql[i] === "$") {
			const tag = /^\$[A-Za-z_]?\w*\$/.exec(sql.slice(i));
			if (tag) {
				const end = sql.indexOf(tag[0], i + tag[0].length);
				i = end === -1 ? sql.length : end + tag[0].length;
			} else {
				out += sql[i];
				i += 1;
			}
		} else {
			out += sql[i];
			i += 1;
		}
	}
	return out;
}

/** Skip a `'…'` literal or `"…"` identifier from the opening quote, past `''`/`""`. */
function skipQuoted(sql: string, start: number, quote: string): number {
	let i = start + 1;
	while (i < sql.length) {
		if (sql[i] === quote) {
			if (sql[i + 1] === quote) {
				i += 2; // a doubled quote is an escaped one, not the end
				continue;
			}
			return i + 1;
		}
		i += 1;
	}
	return sql.length;
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
