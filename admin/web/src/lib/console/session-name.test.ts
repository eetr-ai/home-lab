import { describe, expect, it } from "vitest";
import { DEFAULT_SESSION, sessionName } from "./session-name";

describe("sessionName", () => {
	const tests: { name: string; raw: string | null | undefined; want: string | null }[] = [
		{ name: "absent means the default", raw: null, want: DEFAULT_SESSION },
		{ name: "undefined means the default", raw: undefined, want: DEFAULT_SESSION },
		{ name: "empty means the default", raw: "", want: DEFAULT_SESSION },
		{ name: "a plain name", raw: "deploys", want: "deploys" },
		{ name: "digits, dash and underscore", raw: "s-2_b", want: "s-2_b" },
		{ name: "exactly 32 characters", raw: "a".repeat(32), want: "a".repeat(32) },
		{ name: "33 characters", raw: "a".repeat(33), want: null },
		{ name: "a shell metacharacter", raw: "x;rm", want: null },
		{ name: "a space", raw: "my session", want: null },
		{ name: "a tmux target separator", raw: "a:b", want: null },
		{ name: "a dot", raw: "a.b", want: null },
		{ name: "a trailing newline", raw: "main\n", want: null },
	];

	for (const { name, raw, want } of tests) {
		it(name, () => {
			expect(sessionName(raw)).toBe(want);
		});
	}
});
