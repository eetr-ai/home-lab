import { describe, expect, it } from "vitest";
import { readPickerResult } from "./picker";

const folder = "application/vnd.google-apps.folder";

describe("readPickerResult", () => {
	it("reads a picked folder", () => {
		expect(
			readPickerResult({ action: "picked", docs: [{ id: "abc", name: "Backups", mimeType: folder }] }),
		).toEqual({ id: "abc", name: "Backups" });
	});

	it("treats a cancel as no folder", () => {
		expect(readPickerResult({ action: "cancel" })).toBeNull();
	});

	// The Picker calls back when it loads too, and that is not an answer.
	it("waits through callbacks that are not an answer", () => {
		expect(readPickerResult({ action: "loaded" })).toBe("pending");
		expect(readPickerResult(undefined)).toBe("pending");
	});

	it("never turns a picked file into a folder", () => {
		expect(
			readPickerResult({ action: "picked", docs: [{ id: "abc", mimeType: "text/plain" }] }),
		).toBeNull();
		expect(readPickerResult({ action: "picked", docs: [] })).toBeNull();
		expect(readPickerResult({ action: "picked", docs: [{ id: 7, mimeType: folder }] })).toBeNull();
	});
});
