import { describe, expect, it, vi } from "vitest";
import { refreshTokenSet, type RefreshDeps } from "./refresh";
import type { TokenSet } from "./token-set";

const NOW = 1_700_000_000;

function deps(fetchImpl: RefreshDeps["fetch"]): RefreshDeps {
	return {
		tokenEndpoint: "https://auth.example.invalid/api/token",
		clientId: "admin-panel",
		clientSecret: "s3cret",
		fetch: fetchImpl,
		now: () => NOW,
	};
}

function previous(refreshToken: string): TokenSet {
	return { accessToken: "old-access", refreshToken, expiresAt: NOW - 10 };
}

function jsonResponse(status: number, body: unknown): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { "Content-Type": "application/json" },
	});
}

describe("refreshTokenSet", () => {
	it("exchanges the refresh token and adopts the rotated one", async () => {
		const fetchImpl = vi.fn(async () =>
			jsonResponse(200, {
				access_token: "new-access",
				refresh_token: "new-refresh",
				expires_in: 3600,
			}),
		);

		const got = await refreshTokenSet(previous("rt-1"), deps(fetchImpl));

		expect(got).toEqual({
			ok: true,
			tokens: { accessToken: "new-access", refreshToken: "new-refresh", expiresAt: NOW + 3600 },
		});

		const [url, init] = fetchImpl.mock.calls[0] as unknown as [string, RequestInit];
		expect(url).toBe("https://auth.example.invalid/api/token");
		const headers = new Headers(init.headers);
		// client_secret_basic: the secret travels in the Authorization header, never
		// in the form body, so it stays out of any access log that records one.
		expect(headers.get("Authorization")).toBe(
			`Basic ${Buffer.from("admin-panel:s3cret").toString("base64")}`,
		);
		const form = new URLSearchParams(init.body as string);
		expect(form.get("grant_type")).toBe("refresh_token");
		expect(form.get("refresh_token")).toBe("rt-1");
		expect(form.get("client_secret")).toBeNull();
	});

	it("reports a rejected refresh token instead of throwing", async () => {
		const fetchImpl = vi.fn(async () => jsonResponse(400, { error: "invalid_grant" }));

		const got = await refreshTokenSet(previous("rt-2"), deps(fetchImpl));

		expect(got.ok).toBe(false);
		// Exactly one attempt. eetr-auth rotates refresh tokens with OAuth 2.1 reuse
		// detection, so presenting a rejected one a second time is what cascade-revokes
		// the whole family and signs the operator out everywhere.
		expect(fetchImpl).toHaveBeenCalledTimes(1);
	});

	it("reports a network failure instead of throwing", async () => {
		const fetchImpl = vi.fn(async () => {
			throw new Error("connect ECONNREFUSED");
		});

		const got = await refreshTokenSet(previous("rt-3"), deps(fetchImpl));

		expect(got.ok).toBe(false);
		expect(fetchImpl).toHaveBeenCalledTimes(1);
	});

	it("refuses a session with no refresh token", async () => {
		const fetchImpl = vi.fn(async () => jsonResponse(200, { access_token: "unused" }));

		const got = await refreshTokenSet(
			{ accessToken: "old-access", expiresAt: NOW - 10 },
			deps(fetchImpl),
		);

		expect(got.ok).toBe(false);
		expect(fetchImpl).not.toHaveBeenCalled();
	});

	it("collapses concurrent refreshes of the same token into one exchange", async () => {
		let release!: () => void;
		const gate = new Promise<void>((resolve) => {
			release = resolve;
		});
		const fetchImpl = vi.fn(async () => {
			await gate;
			return jsonResponse(200, {
				access_token: "new-access",
				refresh_token: "new-refresh",
				expires_in: 3600,
			});
		});

		const both = Promise.all([
			refreshTokenSet(previous("rt-4"), deps(fetchImpl)),
			refreshTokenSet(previous("rt-4"), deps(fetchImpl)),
		]);
		release();
		const [first, second] = await both;

		// One exchange, one rotation. Two would present the same refresh token twice
		// and trip reuse detection.
		expect(fetchImpl).toHaveBeenCalledTimes(1);
		expect(first).toEqual(second);
	});

	it("does not collapse refreshes of different tokens", async () => {
		const fetchImpl = vi.fn(async () =>
			jsonResponse(200, { access_token: "new-access", expires_in: 3600 }),
		);

		await Promise.all([
			refreshTokenSet(previous("rt-5"), deps(fetchImpl)),
			refreshTokenSet(previous("rt-6"), deps(fetchImpl)),
		]);

		expect(fetchImpl).toHaveBeenCalledTimes(2);
	});

	it("starts a fresh exchange once the previous one has settled", async () => {
		const fetchImpl = vi.fn(async () =>
			jsonResponse(200, { access_token: "new-access", expires_in: 3600 }),
		);

		await refreshTokenSet(previous("rt-7"), deps(fetchImpl));
		await refreshTokenSet(previous("rt-7"), deps(fetchImpl));

		// The single-flight entry must be cleared when it settles, or the second
		// (legitimate, later) refresh would silently reuse a stale result forever.
		expect(fetchImpl).toHaveBeenCalledTimes(2);
	});
});
