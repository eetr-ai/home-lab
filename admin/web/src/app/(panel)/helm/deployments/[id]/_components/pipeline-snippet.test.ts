import { describe, expect, it } from "vitest";
import { pipelineCurl, pipelineUrl } from "./pipeline-snippet";

describe("pipelineUrl", () => {
	it("addresses the deployment by id", () => {
		expect(pipelineUrl("https://panel.example.invalid", "dep-123")).toBe(
			"https://panel.example.invalid/api/v1/charts/dep-123",
		);
	});

	// AUTH_URL is written by hand in a values file, so one of the two spellings
	// turns up sooner or later and neither may produce a double slash.
	it("tolerates a trailing slash on the origin", () => {
		expect(pipelineUrl("https://panel.example.invalid/", "dep-123")).toBe(
			"https://panel.example.invalid/api/v1/charts/dep-123",
		);
	});

	it("escapes an id that would otherwise change the path", () => {
		expect(pipelineUrl("https://panel.example.invalid", "a/b?c")).toBe(
			"https://panel.example.invalid/api/v1/charts/a%2Fb%3Fc",
		);
	});
});

describe("pipelineCurl", () => {
	// The whole point of the card: this string is pasted into a CI job unedited
	// except for the key, so it is asserted rather than eyeballed.
	it("is a complete request for this deployment", () => {
		expect(pipelineCurl("https://panel.example.invalid", "dep-123", "6.9.4")).toBe(
			[
				'curl -sS -X PATCH "https://panel.example.invalid/api/v1/charts/dep-123" \\',
				'  -H "Authorization: Bearer $EETR_API_KEY" \\',
				"  -H 'Content-Type: application/json' \\",
				`  -d '{"chartVersion":"6.9.4"}'`,
			].join("\n"),
		);
	});

	it("names the key as a variable and never a value", () => {
		const curl = pipelineCurl("https://panel.example.invalid", "dep-123", "6.9.4");
		expect(curl).toContain("$EETR_API_KEY");
		expect(curl).not.toContain("eak_");
	});
});
