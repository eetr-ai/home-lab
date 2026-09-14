import { describe, expect, it } from "vitest";
import { versionField } from "./chart-version-options";

describe("versionField", () => {
	// The regression this exists to hold. An upgrade opens with a current version
	// and nothing asked of any registry, and it has to open as something the
	// operator can type into — a picker holding only the version already in use is
	// a field with no way to change it.
	it("is a text field when no registry has answered, even with a version in hand", () => {
		expect(versionField([], "6.9.1")).toEqual({ picker: false, options: [] });
	});

	it("is a text field when the registry answered with nothing", () => {
		expect(versionField([], "")).toEqual({ picker: false, options: [] });
	});

	it("is a picker once the registry offers versions", () => {
		expect(versionField(["6.9.2", "6.9.1"], "")).toEqual({
			picker: true,
			options: ["6.9.2", "6.9.1"],
		});
	});

	it("lists the registry's versions as they came, when the current one is among them", () => {
		expect(versionField(["6.9.2", "6.9.1"], "6.9.1").options).toEqual(["6.9.2", "6.9.1"]);
	});

	// A chart can drop a version a release is still running. Leaving it out would
	// make the select render as its first option, so the field would show — and
	// then save — a version other than the one it was asked about.
	it("keeps a current version the registry no longer offers, first", () => {
		expect(versionField(["6.9.2"], "6.8.0").options).toEqual(["6.8.0", "6.9.2"]);
	});
});
