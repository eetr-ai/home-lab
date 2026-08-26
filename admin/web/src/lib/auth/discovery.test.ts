import { describe, expect, it } from "vitest";
import { wellKnownUrl } from "./discovery";

describe("wellKnownUrl", () => {
	const tests: { name: string; issuer: string; want: string }[] = [
		{
			name: "plain issuer",
			issuer: "https://auth.example.invalid",
			want: "https://auth.example.invalid/.well-known/openid-configuration",
		},
		// The issuer is stored byte-for-byte, trailing slash and all, because it is
		// an identifier the API compares against. Only the appending trims.
		{
			name: "trailing slash",
			issuer: "https://auth.example.invalid/",
			want: "https://auth.example.invalid/.well-known/openid-configuration",
		},
		{
			name: "several trailing slashes",
			issuer: "https://auth.example.invalid///",
			want: "https://auth.example.invalid/.well-known/openid-configuration",
		},
		{
			name: "issuer with a path",
			issuer: "https://example.invalid/tenants/eetr",
			want: "https://example.invalid/tenants/eetr/.well-known/openid-configuration",
		},
	];

	for (const { name, issuer, want } of tests) {
		it(name, () => {
			expect(wellKnownUrl(issuer)).toBe(want);
		});
	}

	it("refuses an unset issuer rather than building a nonsense URL", () => {
		expect(() => wellKnownUrl("")).toThrow(/OIDC_ISSUER/);
	});
});
