import { describe, expect, it } from "vitest";
import { digestToken, openOutcome, sealOutcome } from "./outcome-seal";
import type { RefreshOutcome } from "./refresh";

const SECRET = "an-auth-secret-of-some-length";

const OK: RefreshOutcome = {
	ok: true,
	tokens: { accessToken: "new-access", refreshToken: "new-refresh", expiresAt: 1_700_000_000 },
};

describe("sealOutcome / openOutcome", () => {
	it("round-trips an outcome", async () => {
		const opened = await openOutcome(await sealOutcome(OK, SECRET), SECRET);
		expect(opened).toEqual(OK);
	});

	it("round-trips a failure, which is cached as deliberately as a success", async () => {
		const failure: RefreshOutcome = { ok: false, error: "token refresh rejected (400)" };
		expect(await openOutcome(await sealOutcome(failure, SECRET), SECRET)).toEqual(failure);
	});

	// The reason this is encrypted at all: what goes to the shared store must not
	// be the token pair in the clear.
	it("does not leave the tokens readable in the sealed value", async () => {
		const sealed = await sealOutcome(OK, SECRET);
		expect(sealed).not.toContain("new-access");
		expect(sealed).not.toContain("new-refresh");
	});

	// A rotated AUTH_SECRET, or two deployments sharing one Redis. Null rather than
	// a throw, because the caller recovers by treating it as a cache miss.
	it("refuses a value sealed under a different secret", async () => {
		const sealed = await sealOutcome(OK, SECRET);
		expect(await openOutcome(sealed, "a-different-secret")).toBeNull();
	});

	// AES-GCM is authenticated, so this is the assertion that says so: a value
	// somebody edited must not decrypt into something that is then trusted.
	it("refuses a tampered value", async () => {
		const sealed = await sealOutcome(OK, SECRET);
		const bytes = atob(sealed).split("");
		bytes[bytes.length - 1] = String.fromCharCode(
			bytes[bytes.length - 1].charCodeAt(0) ^ 0xff,
		);
		expect(await openOutcome(btoa(bytes.join("")), SECRET)).toBeNull();
	});

	it("refuses values that are truncated, empty or not base64 at all", async () => {
		expect(await openOutcome("", SECRET)).toBeNull();
		expect(await openOutcome("not base64 !!", SECRET)).toBeNull();
		expect(await openOutcome(btoa("short"), SECRET)).toBeNull();
	});

	// The nonce is per value, so two seals of the same outcome must differ —
	// otherwise the store leaks which sessions refreshed to the same result.
	it("seals the same outcome differently each time", async () => {
		expect(await sealOutcome(OK, SECRET)).not.toBe(await sealOutcome(OK, SECRET));
	});

	// Shape-checked on the way out: this value came from a shared store, and an
	// outcome that is neither a success nor a failure would reach the session as
	// undefined tokens.
	it("refuses a well-formed value that is not an outcome", async () => {
		const sealed = await sealOutcome({ ok: true } as unknown as RefreshOutcome, SECRET);
		expect(await openOutcome(sealed, SECRET)).toBeNull();
	});
});

describe("digestToken", () => {
	it("is stable for a token and different between tokens", async () => {
		expect(await digestToken("rt-1")).toBe(await digestToken("rt-1"));
		expect(await digestToken("rt-1")).not.toBe(await digestToken("rt-2"));
	});

	// Keys are the part of a store that leaks most readily — MONITOR streams them,
	// SLOWLOG samples them, KEYS lists them.
	it("does not contain the token", async () => {
		expect(await digestToken("super-secret-token")).not.toContain("super-secret-token");
	});
});
