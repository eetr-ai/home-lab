import { describe, expect, it } from "vitest";
import {
	DEFAULT_LIFETIME_SECONDS,
	REFRESH_SKEW_SECONDS,
	needsRefresh,
	nextTokenSet,
	type TokenSet,
} from "./token-set";

const NOW = 1_700_000_000;

describe("needsRefresh", () => {
	const tests: { name: string; expiresAt: number | undefined; want: boolean }[] = [
		{ name: "comfortably valid", expiresAt: NOW + 3600, want: false },
		{ name: "just outside the skew window", expiresAt: NOW + REFRESH_SKEW_SECONDS + 1, want: false },
		// Inside the skew window the token is still technically valid, but a request
		// made now could arrive after it expires — so it counts as needing a refresh.
		{ name: "inside the skew window", expiresAt: NOW + REFRESH_SKEW_SECONDS - 1, want: true },
		{ name: "expired", expiresAt: NOW - 1, want: true },
		// An unknown expiry is treated as expired rather than as valid: guessing
		// "still good" means sending a dead token and failing the request.
		{ name: "unknown", expiresAt: undefined, want: true },
	];

	for (const { name, expiresAt, want } of tests) {
		it(name, () => {
			expect(needsRefresh(expiresAt, NOW)).toBe(want);
		});
	}
});

describe("nextTokenSet", () => {
	const previous: TokenSet = {
		accessToken: "old-access",
		refreshToken: "old-refresh",
		expiresAt: NOW - 10,
	};

	it("adopts the rotated refresh token", () => {
		const got = nextTokenSet(
			previous,
			{ access_token: "new-access", refresh_token: "new-refresh", expires_in: 3600 },
			NOW,
		);
		expect(got).toEqual({
			accessToken: "new-access",
			refreshToken: "new-refresh",
			expiresAt: NOW + 3600,
		});
	});

	it("keeps the previous refresh token when the response omits one", () => {
		const got = nextTokenSet(previous, { access_token: "new-access", expires_in: 60 }, NOW);
		expect(got.refreshToken).toBe("old-refresh");
	});

	it("falls back to a default lifetime when expires_in is missing", () => {
		const got = nextTokenSet(previous, { access_token: "new-access" }, NOW);
		expect(got.expiresAt).toBe(NOW + DEFAULT_LIFETIME_SECONDS);
	});

	it("ignores a non-numeric expires_in", () => {
		const got = nextTokenSet(
			previous,
			{ access_token: "new-access", expires_in: "3600" as unknown as number },
			NOW,
		);
		expect(got.expiresAt).toBe(NOW + DEFAULT_LIFETIME_SECONDS);
	});
});
