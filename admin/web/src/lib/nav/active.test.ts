import { describe, expect, it } from "vitest";
import { activeHref } from "./active";

const tabs = ["/postgres/databases", "/postgres/roles", "/postgres"];

describe("activeHref", () => {
	const tests: { name: string; hrefs: string[]; pathname: string; want?: string }[] = [
		{ name: "exact match", hrefs: tabs, pathname: "/postgres/roles", want: "/postgres/roles" },
		{
			name: "a child route keeps its tab active",
			hrefs: tabs,
			pathname: "/postgres/roles/reporting",
			want: "/postgres/roles",
		},
		// The parent is a prefix of the child, so a first-match rule would light up
		// the section while the operator is inside one of its tabs.
		{ name: "the longest match wins", hrefs: tabs, pathname: "/postgres/databases", want: "/postgres/databases" },
		{ name: "the section itself", hrefs: tabs, pathname: "/postgres", want: "/postgres" },
		{
			name: "a sibling that merely starts with the same characters",
			hrefs: ["/postgres"],
			pathname: "/postgres-archive",
			want: undefined,
		},
		{ name: "nothing matches", hrefs: tabs, pathname: "/mongo", want: undefined },
		{ name: "no entries", hrefs: [], pathname: "/postgres", want: undefined },
		// The root is a prefix of every path, so it must only ever match itself.
		{ name: "the root does not swallow everything", hrefs: ["/", "/overview"], pathname: "/overview", want: "/overview" },
	];

	for (const { name, hrefs, pathname, want } of tests) {
		it(name, () => {
			expect(activeHref(hrefs, pathname)).toBe(want);
		});
	}
});
