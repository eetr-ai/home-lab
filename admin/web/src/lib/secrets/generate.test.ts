import { describe, expect, it } from "vitest";
import { MAX_LENGTH, MIN_LENGTH, PRESETS, isValidLength } from "./generate";

// How a value is made is tested in Go, where it is made:
// admin/api/internal/secretgen. What is left here is what the form renders.

describe("PRESETS", () => {
	// The ids are the `shape` values the API takes. A typo here is a 400 the
	// operator sees as a broken button.
	it("names only shapes the API accepts", () => {
		expect(PRESETS.map((preset) => preset.id)).toEqual([
			"password",
			"alphanumeric",
			"hex",
			"base64",
		]);
	});

	// A sized preset renders a length field and a fixed one does not. The token
	// shapes are 256 bits because that is the requirement, so offering a length
	// there would invite somebody to shorten a signing key.
	it("marks the token shapes as fixed-size", () => {
		const sized = Object.fromEntries(PRESETS.map((preset) => [preset.id, preset.sized]));
		expect(sized).toEqual({ password: true, alphanumeric: true, hex: false, base64: false });
	});
});

describe("isValidLength", () => {
	it("accepts what the API will", () => {
		for (const length of [MIN_LENGTH, 24, 64, MAX_LENGTH]) {
			expect(isValidLength(length), String(length)).toBe(true);
		}
	});

	it("refuses what the API would refuse, before asking it", () => {
		for (const length of [0, -8, MIN_LENGTH - 1, MAX_LENGTH + 1, 24.5, Number.NaN]) {
			expect(isValidLength(length), String(length)).toBe(false);
		}
	});
});
