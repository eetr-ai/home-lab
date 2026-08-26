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

if (failures > 0) {
	console.error(`\n${failures} theme violation(s). See docs/contributing/ux-guidelines.md › Theme.\n`);
	process.exit(1);
}

console.log("✓ theme: no raw color ramps or legacy radii outside theme.css");
