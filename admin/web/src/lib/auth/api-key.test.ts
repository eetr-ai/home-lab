import { describe, expect, it, vi } from "vitest";
import { OAuthError } from "@eetr/eetr-auth-client";
import { apiKeyEndpoint, apiKeyFrom, exchangeForToken, type ExchangeDeps } from "./api-key";

const KEY = "eak_0123abcd_thesecretpart";

/** The refusal every rejected key gets, spelled once here as it is in the source. */
const REFUSED = "the API key was not accepted";

function deps(exchange: ExchangeDeps["exchange"], audience = ""): ExchangeDeps {
	return { exchange, issuer: "https://auth.example.invalid", audience };
}

function token(access_token: string) {
	return vi.fn(async () => ({ access_token, token_type: "Bearer", expires_in: 3600 }));
}

describe("apiKeyFrom", () => {
	it("reads the key out of a bearer header", () => {
		expect(apiKeyFrom(`Bearer ${KEY}`)).toBe(KEY);
	});

	it("accepts the scheme in any case, as HTTP does", () => {
		expect(apiKeyFrom(`bearer ${KEY}`)).toBe(KEY);
		expect(apiKeyFrom(`BEARER ${KEY}`)).toBe(KEY);
	});

	it.each([
		["absent", null],
		["empty", ""],
		["schemeless", KEY],
		["basic", "Basic dXNlcjpwYXNz"],
		["a bearer that is not an API key", "Bearer eyJhbGciOiJSUzI1NiJ9.e30.sig"],
		["the prefix and nothing else", "Bearer eak_"],
		["two tokens", `Bearer ${KEY} extra`],
	])("returns nothing for a header that is %s", (_name, header) => {
		expect(apiKeyFrom(header)).toBeUndefined();
	});
});

describe("apiKeyEndpoint", () => {
	// The issuer is stored byte-for-byte because the API compares tokens against
	// it. Both spellings have to produce one URL, or half the labs configured this
	// way would post to a double slash.
	it.each([
		["https://auth.example.invalid", "https://auth.example.invalid/api/token/api-key"],
		["https://auth.example.invalid/", "https://auth.example.invalid/api/token/api-key"],
		["https://auth.example.invalid///", "https://auth.example.invalid/api/token/api-key"],
	])("derives %s into %s", (issuer, expected) => {
		expect(apiKeyEndpoint(issuer)).toBe(expected);
	});
});

describe("exchangeForToken", () => {
	it("returns the access token", async () => {
		const exchange = token("an.access.token");
		await expect(exchangeForToken(KEY, deps(exchange))).resolves.toEqual({
			token: "an.access.token",
		});
		expect(exchange).toHaveBeenCalledWith(
			{ apiKey: KEY },
			{ apiKeyEndpoint: "https://auth.example.invalid/api/token/api-key" },
		);
	});

	it("asks for the audience as a resource when one is configured", async () => {
		const exchange = token("an.access.token");
		await exchangeForToken(KEY, deps(exchange, "admin-panel-client-id"));
		expect(exchange).toHaveBeenCalledWith(
			{ apiKey: KEY, resource: "admin-panel-client-id" },
			expect.anything(),
		);
	});

	// Not a formality: asking for a resource the provider does not know about is a
	// way to fail an exchange that would otherwise have worked.
	it("asks for no resource when the audience is unset", async () => {
		const exchange = token("an.access.token");
		await exchangeForToken(KEY, deps(exchange));
		expect(exchange).toHaveBeenCalledWith(
			expect.not.objectContaining({ resource: expect.anything() }),
			expect.anything(),
		);
	});

	it("never asks for a scope", async () => {
		const exchange = token("an.access.token");
		await exchangeForToken(KEY, deps(exchange, "aud"));
		expect(exchange).toHaveBeenCalledWith(
			expect.not.objectContaining({ scope: expect.anything(), scopes: expect.anything() }),
			expect.anything(),
		);
	});

	it("refuses a key the provider rejected, saying no more than it does", async () => {
		const exchange = vi.fn(async () => {
			throw new OAuthError("invalid_client", "API key exchange failed: 401", 401);
		});
		await expect(exchangeForToken(KEY, deps(exchange))).resolves.toEqual({
			error: REFUSED,
			status: 401,
		});
	});

	// The one distinction worth drawing. A provider that never answered says
	// nothing about the key, and reporting it as a bad key sends somebody to
	// rotate a credential that was fine.
	it("reports an unreachable provider as itself, not as a bad key", async () => {
		const exchange = vi.fn(async () => {
			throw new TypeError("fetch failed");
		});
		const result = await exchangeForToken(KEY, deps(exchange));
		expect(result).toEqual({
			error: "the identity provider is unreachable: fetch failed",
			status: 502,
		});
	});

	it("refuses an answer that carried no token", async () => {
		const exchange = vi.fn(async () => ({ access_token: "", token_type: "Bearer" }));
		await expect(exchangeForToken(KEY, deps(exchange))).resolves.toEqual({
			error: REFUSED,
			status: 401,
		});
	});

	it("does not reach out at all when the issuer is unconfigured", async () => {
		const exchange = token("an.access.token");
		const result = await exchangeForToken(KEY, { exchange, issuer: "", audience: "" });
		expect(result).toEqual({
			error: "the identity provider is not configured (OIDC_ISSUER is unset)",
			status: 502,
		});
		expect(exchange).not.toHaveBeenCalled();
	});
});
