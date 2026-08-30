import { describe, expect, it } from "vitest";
import { parsePatch } from "./patch-request";

describe("parsePatch", () => {
	it("translates chartVersion into the API's version", () => {
		expect(parsePatch({ chartVersion: "6.9.4" })).toEqual({ request: { version: "6.9.4" } });
	});

	it("translates valueOverrides into the API's values", () => {
		const result = parsePatch({
			chartVersion: "6.9.4",
			valueOverrides: { image: { tag: "sha-abc123" } },
		});
		expect(result).toEqual({
			request: { version: "6.9.4", values: { image: { tag: "sha-abc123" } } },
		});
	});

	// The difference the API acts on: no `values` key carries the previous document
	// forward untouched, where an empty one regenerates it and loses the comments.
	it("omits values entirely rather than sending an empty object", () => {
		const result = parsePatch({ chartVersion: "6.9.4" });
		expect("request" in result && "values" in result.request).toBe(false);
	});

	it("keeps an explicitly empty valueOverrides", () => {
		expect(parsePatch({ chartVersion: "6.9.4", valueOverrides: {} })).toEqual({
			request: { version: "6.9.4", values: {} },
		});
	});

	it("does not carry rollbackOnFailure through", () => {
		const result = parsePatch({ chartVersion: "6.9.4", rollbackOnFailure: true });
		expect(result).toEqual({ request: { version: "6.9.4" } });
	});

	it.each([
		["missing", {}],
		["blank", { chartVersion: "" }],
		["whitespace", { chartVersion: "   " }],
		["not a string", { chartVersion: 6.9 }],
		["null", { chartVersion: null }],
	])("refuses a chartVersion that is %s", (_name, body) => {
		expect(parsePatch(body)).toEqual({
			error: "chartVersion is required and must be a non-empty string",
		});
	});

	it.each([
		["an array", []],
		["a string", "hello"],
		["a number", 3],
	])("refuses valueOverrides that is %s", (_name, overrides) => {
		expect(parsePatch({ chartVersion: "6.9.4", valueOverrides: overrides })).toEqual({
			error: "valueOverrides must be a JSON object",
		});
	});

	it("treats a null valueOverrides as absent", () => {
		expect(parsePatch({ chartVersion: "6.9.4", valueOverrides: null })).toEqual({
			request: { version: "6.9.4" },
		});
	});

	// Anything JSON can decode to reaches this function, and none of it may throw.
	it.each([
		["null", null],
		["an array", []],
		["a number", 42],
		["a string", "6.9.4"],
		["a boolean", true],
	])("refuses a body that is %s", (_name, body) => {
		expect(parsePatch(body)).toEqual({ error: "the body must be a JSON object" });
	});
});
