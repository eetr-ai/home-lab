import { encode } from "@auth/core/jwt";
import { describe, expect, it } from "vitest";
import { authorize, type GateConfig } from "./authorize";

const SECRET = "a-test-secret-that-is-long-enough-for-hkdf-0123456789";
const PANEL = "https://admin.example.invalid";
const COOKIE = "__Secure-authjs.session-token";

const config: GateConfig = {
	secret: SECRET,
	panelUrl: PANEL,
	allowlist: [],
	allowAnyOperator: true,
};

/** A session cookie exactly as Auth.js would seal it. */
async function seal(
	claims: Record<string, unknown>,
	opts: { secret?: string; name?: string; maxAge?: number } = {},
): Promise<string> {
	const name = opts.name ?? COOKIE;
	const value = await encode({
		token: claims,
		secret: opts.secret ?? SECRET,
		salt: name,
		maxAge: opts.maxAge,
	});
	return `${name}=${value}`;
}

function request(cookie: string, origin: string | null = PANEL) {
	return { cookie, origin: origin ?? undefined, host: "admin.example.invalid" };
}

describe("authorize", () => {
	it("admits a signed-in operator from the panel", async () => {
		const cookie = await seal({ email: "op@example.invalid", sub: "1" });
		expect(await authorize(request(cookie), config)).toEqual({
			ok: true,
			email: "op@example.invalid",
		});
	});

	it("refuses another origin, cookie or not", async () => {
		const cookie = await seal({ email: "op@example.invalid" });
		const verdict = await authorize(request(cookie, "https://evil.example.invalid"), config);
		expect(verdict).toMatchObject({ ok: false, status: 403 });
	});

	it("refuses a request with no origin at all", async () => {
		const cookie = await seal({ email: "op@example.invalid" });
		expect(await authorize(request(cookie, null), config)).toMatchObject({
			ok: false,
			status: 403,
		});
	});

	it("refuses no session", async () => {
		expect(await authorize(request(""), config)).toMatchObject({ ok: false, status: 401 });
	});

	it("refuses a cookie sealed under another secret", async () => {
		const cookie = await seal({ email: "op@example.invalid" }, { secret: `${SECRET}-other` });
		expect(await authorize(request(cookie), config)).toMatchObject({ ok: false, status: 401 });
	});

	it("refuses an expired session", async () => {
		const cookie = await seal({ email: "op@example.invalid" }, { maxAge: -60 });
		expect(await authorize(request(cookie), config)).toMatchObject({ ok: false, status: 401 });
	});

	it("refuses a session whose refresh failed", async () => {
		const cookie = await seal({ email: "op@example.invalid", error: "RefreshFailed" });
		expect(await authorize(request(cookie), config)).toMatchObject({ ok: false, status: 401 });
	});

	it("reads the plain cookie name when the panel is served over http", async () => {
		const cookie = await seal({ email: "op@example.invalid" }, { name: "authjs.session-token" });
		const local = { ...config, panelUrl: "http://localhost:3000" };
		expect(await authorize(request(cookie, "http://localhost:3000"), local)).toMatchObject({
			ok: true,
		});
		// …and does not read the https one there, nor the http one over https.
		expect(await authorize(request(cookie), config)).toMatchObject({ ok: false, status: 401 });
	});

	it("reassembles a cookie Auth.js split into chunks", async () => {
		const whole = await seal({ email: "op@example.invalid", padding: "x".repeat(6000) });
		const value = whole.slice(COOKIE.length + 1);
		const half = Math.ceil(value.length / 2);
		const chunked = `${COOKIE}.0=${value.slice(0, half)}; ${COOKIE}.1=${value.slice(half)}`;
		expect(await authorize(request(chunked), config)).toMatchObject({ ok: true });
	});

	describe("with a write allowlist", () => {
		const narrowed = { ...config, allowlist: ["op@example.invalid"] };

		it("admits an operator on it", async () => {
			const cookie = await seal({ email: "OP@example.invalid" });
			expect(await authorize(request(cookie), narrowed)).toMatchObject({ ok: true });
		});

		it("refuses a signed-in operator who is not", async () => {
			const cookie = await seal({ email: "reader@example.invalid" });
			expect(await authorize(request(cookie), narrowed)).toMatchObject({
				ok: false,
				status: 403,
			});
		});

		it("refuses a session with no email", async () => {
			const cookie = await seal({ sub: "1" });
			expect(await authorize(request(cookie), narrowed)).toMatchObject({
				ok: false,
				status: 403,
			});
		});
	});

	describe("with no allowlist", () => {
		it("is closed unless the install opted every operator in", async () => {
			const cookie = await seal({ email: "op@example.invalid" });
			const closed = { ...config, allowAnyOperator: false };
			const verdict = await authorize(request(cookie), closed);
			expect(verdict).toMatchObject({ ok: false, status: 403 });
			if (!verdict.ok) expect(verdict.reason).toContain("ADMIN_WRITE_EMAILS");
		});

		it("admits any signed-in operator once opted in", async () => {
			const cookie = await seal({ email: "op@example.invalid" });
			expect(await authorize(request(cookie), config)).toMatchObject({ ok: true });
		});
	});
});
