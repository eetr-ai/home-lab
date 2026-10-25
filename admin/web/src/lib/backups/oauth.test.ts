import { describe, expect, it } from "vitest";
import {
	authorizationUrl,
	challengeFor,
	FLOW_TTL_SECONDS,
	newFlow,
	openFlow,
	redirectUriFor,
	sameState,
	sealFlow,
} from "./oauth";

const secret = "an-auth-secret-for-tests";

describe("newFlow", () => {
	it("makes a verifier inside RFC 7636's 43 to 128 characters", () => {
		const { verifier } = newFlow(0);
		expect(verifier.length).toBeGreaterThanOrEqual(43);
		expect(verifier.length).toBeLessThanOrEqual(128);
		expect(verifier).toMatch(/^[A-Za-z0-9_-]+$/);
	});

	it("never repeats a state", () => {
		expect(newFlow(0).state).not.toBe(newFlow(0).state);
	});
});

describe("challengeFor", () => {
	// The example from RFC 7636 Appendix B, so this is checked against the spec
	// and not against itself.
	it("computes the S256 challenge the RFC does", async () => {
		await expect(challengeFor("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")).resolves.toBe(
			"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
		);
	});
});

describe("authorizationUrl", () => {
	it("asks for offline access, fresh consent, and PKCE", () => {
		const url = new URL(
			authorizationUrl({
				clientId: "client",
				redirectUri: "https://panel.example.invalid/api/backups/google/callback",
				scopes: ["https://www.googleapis.com/auth/drive.file", "openid", "email"],
				state: "s",
				challenge: "c",
			}),
		);
		expect(url.origin + url.pathname).toBe("https://accounts.google.com/o/oauth2/v2/auth");
		expect(url.searchParams.get("access_type")).toBe("offline");
		expect(url.searchParams.get("prompt")).toBe("consent");
		expect(url.searchParams.get("code_challenge_method")).toBe("S256");
		expect(url.searchParams.get("scope")).toBe(
			"https://www.googleapis.com/auth/drive.file openid email",
		);
	});
});

describe("sealFlow and openFlow", () => {
	it("round-trips a live flow", async () => {
		const flow = newFlow(1000);
		await expect(openFlow(await sealFlow(flow, secret), secret, 1000)).resolves.toEqual(flow);
	});

	it("refuses a flow older than its ten minutes", async () => {
		const sealed = await sealFlow(newFlow(0), secret);
		await expect(openFlow(sealed, secret, FLOW_TTL_SECONDS * 1000 + 1)).resolves.toBeNull();
	});

	it("refuses one sealed under another secret", async () => {
		const sealed = await sealFlow(newFlow(0), secret);
		await expect(openFlow(sealed, "another-secret", 0)).resolves.toBeNull();
	});

	it("refuses a tampered or truncated value without throwing", async () => {
		const sealed = await sealFlow(newFlow(0), secret);
		const flipped = (sealed[20] === "A" ? "B" : "A");
		for (const bad of [sealed.slice(0, 10), sealed.slice(0, 20) + flipped + sealed.slice(21), "", "%%%"]) {
			await expect(openFlow(bad, secret, 0)).resolves.toBeNull();
		}
	});
});

describe("sameState", () => {
	it("accepts only the exact state", () => {
		expect(sameState("abc", "abc")).toBe(true);
		expect(sameState("abc", "abd")).toBe(false);
		expect(sameState("abc", "ab")).toBe(false);
		expect(sameState("abc", null)).toBe(false);
	});
});

describe("redirectUriFor", () => {
	it("uses AUTH_URL's origin, whatever follows it", () => {
		for (const authUrl of [
			"https://panel.example.invalid",
			"https://panel.example.invalid/",
			"https://panel.example.invalid/some/path",
		]) {
			expect(redirectUriFor(authUrl)).toBe("https://panel.example.invalid/api/backups/google/callback");
		}
	});

	it("refuses to guess without a URL", () => {
		expect(redirectUriFor(undefined)).toBeNull();
		expect(redirectUriFor("panel.example.invalid")).toBeNull();
	});
});
