import { describe, expect, it } from "vitest";
import { displayText, formatTtl, keyId, keyQueryValue } from "./text";

describe("keyQueryValue", () => {
	it("percent-encodes a text key, the characters a query treats specially included", () => {
		// An & or = left raw would end the value early or start a second parameter.
		expect(keyQueryValue({ value: "lock:a/b c&d=e", encoding: "utf8" })).toBe("lock%3Aa%2Fb%20c%26d%3De");
	});

	it("leaves dot keys intact, which a path segment could not", () => {
		expect(keyQueryValue({ value: "..", encoding: "utf8" })).toBe("..");
	});

	it("encodes UTF-8 text as its bytes", () => {
		expect(keyQueryValue({ value: "ñ", encoding: "utf8" })).toBe("%C3%B1");
	});

	it("turns a base64 key back into its raw bytes, one escape per byte", () => {
		// 0xFF 0xFE is not valid UTF-8, which is why the API sent it base64. Run
		// through encodeURIComponent as a string it would come out as %C3%BF%C3%BE
		// — four bytes, a different key.
		expect(keyQueryValue({ value: "//4=", encoding: "base64" })).toBe("%FF%FE");
	});
});

describe("keyId", () => {
	it("tells a text key from a base64 key with the same characters", () => {
		expect(keyId({ value: "YQ==", encoding: "utf8" })).not.toBe(
			keyId({ value: "YQ==", encoding: "base64" }),
		);
	});
});

describe("displayText", () => {
	it("marks base64 rather than pretending it is text", () => {
		// Not "base64://4=", which reads as a URL.
		expect(displayText({ value: "//4=", encoding: "base64" })).toBe("(base64) //4=");
		expect(displayText({ value: "plain", encoding: "utf8" })).toBe("plain");
	});
});

describe("formatTtl", () => {
	it.each([
		[-1, "never"],
		[250, "250ms"],
		[45_000, "45s"],
		[125_000, "2m 5s"],
		[3_660_000, "1h 1m"],
		[90_000_000, "1d 1h"],
	])("%i ms reads as %s", (millis, text) => {
		expect(formatTtl(millis)).toBe(text);
	});
});
