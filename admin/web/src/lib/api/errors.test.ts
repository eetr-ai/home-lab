import { describe, expect, it } from "vitest";
import { errorMessage } from "./errors";

describe("errorMessage", () => {
	const tests: { name: string; body: unknown; status: number; want: string }[] = [
		{
			name: "prefers the human-readable message",
			body: { error: "conflict", message: "a database with that name already exists" },
			status: 409,
			want: "a database with that name already exists",
		},
		{
			name: "falls back to the machine-readable code",
			body: { error: "invalid_request" },
			status: 400,
			want: "invalid_request",
		},
		{
			name: "explains a status the operator can act on",
			body: null,
			status: 401,
			want: "the session is no longer accepted by the API — sign in again",
		},
		{
			name: "names the status when there is nothing else",
			body: null,
			status: 500,
			want: "the admin API returned 500",
		},
		// A body that is not an object at all must not throw while being read.
		{ name: "a bare array body", body: [], status: 500, want: "the admin API returned 500" },
		{ name: "a numeric body", body: 7, status: 500, want: "the admin API returned 500" },
		{
			name: "an empty message is not a message",
			body: { error: "conflict", message: "" },
			status: 409,
			want: "conflict",
		},
	];

	for (const { name, body, status, want } of tests) {
		it(name, () => {
			expect(errorMessage(body, status)).toBe(want);
		});
	}
});
