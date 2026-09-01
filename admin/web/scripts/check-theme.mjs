#!/usr/bin/env node
/**
 * Theme drift guard.
 *
 * The panel has a two-tier theme (src/app/theme.css): a palette, and semantic
 * roles that components reference. That only stays true if raw utilities cannot
 * creep back in, so fail the build when they do.
 *
 * Run via `npm run lint` (or `task admin-web:lint`), or directly:
 *   node scripts/check-theme.mjs
 */
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const appRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const SCAN = ["src/app", "src/components"];

/** [pattern, why, suggested replacement] */
const BANNED = [
	[
		String.raw`\b(hover:|group-hover:|focus:|dark:)*(text|bg|border|divide|ring)-(red|green|amber|blue|emerald|slate|zinc|gray|neutral)-[0-9]{2,3}\b`,
		"raw Tailwind color ramp",
		"use a role token (danger/success/warning/accent, or border/surface/foreground)",
	],
	[
		String.raw`\brounded-xl\b`,
		"legacy container radius",
		"use rounded-card (or rounded-control / rounded-chip)",
	],
	[
		String.raw`\bborder-brand-muted\b`,
		"brand tint used as a default border",
		"use border-border (or border-border-strong for emphasis)",
	],
];

// theme.css is where the palette legitimately lives.
const EXCLUDE = /theme\.css$/;

let failures = 0;

for (const [pattern, why, fix] of BANNED) {
	let out = "";
	try {
		out = execFileSync(
			"grep",
			["-rnE", "--include=*.tsx", "--include=*.ts", "--include=*.css", pattern, ...SCAN],
			{ cwd: appRoot, encoding: "utf8" },
		);
	} catch (err) {
		// grep exits 1 when there are no matches, which is the success case here.
		if (err.status === 1) continue;
		throw err;
	}

	const hits = out
		.split("\n")
		.filter(Boolean)
		.filter((line) => !EXCLUDE.test(line.split(":")[0]));

	if (hits.length === 0) continue;

	failures += hits.length;
	console.error(`\n✖ ${why} (${hits.length}) — ${fix}`);
	for (const hit of hits.slice(0, 20)) console.error(`  ${hit.trim().slice(0, 160)}`);
	if (hits.length > 20) console.error(`  …and ${hits.length - 20} more`);
}

// --- roles that do not exist -------------------------------------------------
//
// The rules above ban the wrong kind of colour. This one catches the colour that
// is not a colour: `text-danger` looks exactly like a role token and there is no
// --color-danger, only danger-fg, danger-bg, danger-border and danger-icon. It
// does not fail, it does not warn, and Tailwind emits nothing — so the text
// renders in the inherited colour, which for an error line is the one outcome
// that must not happen quietly.
//
// It slipped in twice before this check existed, both times on an error message,
// where "looks like normal text" is exactly what nobody notices in review.
//
// Scoped to names that are TRYING to be a role: the first segment matches a
// family the theme defines (danger-, surface-, brand-…) but the whole name does
// not. That is what separates `text-danger` from `text-xs`, which is a size and
// has nothing to do with --color-*. A blunter rule flags 268 things and gets
// switched off within a week.
const roles = new Set(
	[
		...readFileSync(path.join(appRoot, "src/app/theme.css"), "utf8")
			.matchAll(/--color-([a-z0-9-]+)\s*:/g),
	].map((match) => match[1]),
);
const families = new Set([...roles].filter((role) => role.includes("-")).map((role) => role.split("-")[0]));

const COLOR_PREFIXES = "text|bg|border|divide|ring|fill|stroke";

let unknown = 0;
try {
	const out = execFileSync(
		"grep",
		[
			"-rnoE",
			"--include=*.tsx",
			"--include=*.ts",
			String.raw`\b(?:[a-z-]+:)*(?:${COLOR_PREFIXES})-[a-z][a-z0-9-]*\b`,
			...SCAN,
		],
		{ cwd: appRoot, encoding: "utf8" },
	);

	for (const line of out.split("\n").filter(Boolean)) {
		const file = line.split(":")[0];
		if (EXCLUDE.test(file) || /\.test\.tsx?$/.test(file)) continue;

		const utility = line.slice(line.lastIndexOf(":") + 1);
		const role = utility
			.replace(/^(?:[a-z-]+:)*/, "")
			.replace(new RegExp(`^(?:${COLOR_PREFIXES})-`), "");

		if (roles.has(role) || !families.has(role.split("-")[0])) continue;

		const near = [...roles].filter((known) => known.startsWith(role.split("-")[0] + "-"));
		unknown += 1;
		console.error(
			`\n✖ no such theme role: ${utility} — ${file}` +
				(near.length
					? `\n  the theme defines ${near.join(", ")}; this one resolves to nothing`
					: ""),
		);
	}
} catch (err) {
	if (err.status !== 1) throw err;
}
failures += unknown;

if (failures > 0) {
	console.error(`\n${failures} theme violation(s). See docs/contributing/ux-guidelines.md › Theme.\n`);
	process.exit(1);
}

console.log("✓ theme: no raw ramps, no legacy radii, and every role token exists");
