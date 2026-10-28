import type { ClipboardSelectionType } from "@xterm/addon-clipboard";
import { describe, expect, it } from "vitest";
import { writeOnlyClipboard } from "./clipboard";

/** A clipboard that remembers what it was given, holding a secret to start with. */
function fakeClipboard() {
	const written: string[] = [];
	return {
		written,
		writeText: async (text: string) => {
			written.push(text);
		},
		readText: async () => "a secret the shell must not see",
	};
}

const as = (selection: string) => selection as ClipboardSelectionType;

describe("writeOnlyClipboard", () => {
	it("copies what tmux sends, which names no selection at all", async () => {
		const clipboard = fakeClipboard();
		await writeOnlyClipboard(clipboard).writeText(as(""), "kubectl get pods");
		expect(clipboard.written).toEqual(["kubectl get pods"]);
	});

	it("copies to the explicit clipboard selection", async () => {
		const clipboard = fakeClipboard();
		await writeOnlyClipboard(clipboard).writeText(as("c"), "hello");
		expect(clipboard.written).toEqual(["hello"]);
	});

	it("ignores X selections the browser has no equivalent for", async () => {
		const clipboard = fakeClipboard();
		await writeOnlyClipboard(clipboard).writeText(as("p"), "primary");
		expect(clipboard.written).toEqual([]);
	});

	it("never reads the clipboard back to the shell", async () => {
		const provider = writeOnlyClipboard(fakeClipboard());
		expect(await provider.readText(as("c"))).toBe("");
		expect(await provider.readText(as(""))).toBe("");
	});

	it("shrugs off a clipboard that refuses, or is not there", async () => {
		const refusing = { writeText: () => Promise.reject(new Error("NotAllowedError")) };
		await expect(writeOnlyClipboard(refusing).writeText(as(""), "x")).resolves.toBeUndefined();
		await expect(writeOnlyClipboard(undefined).writeText(as(""), "x")).resolves.toBeUndefined();
	});
});
