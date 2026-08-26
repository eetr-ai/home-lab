import { afterEach, describe, expect, it, vi } from "vitest";
import { randomId, readThreadId, threadKey } from "./thread";

/** Swap in a crypto with only the capabilities named, and put the real one back. */
function withCrypto(replacement: unknown) {
	const original = Object.getOwnPropertyDescriptor(globalThis, "crypto");
	Object.defineProperty(globalThis, "crypto", { value: replacement, configurable: true });
	return () => {
		if (original) Object.defineProperty(globalThis, "crypto", original);
	};
}

let restore: (() => void) | undefined;
afterEach(() => {
	restore?.();
	restore = undefined;
	vi.unstubAllGlobals();
});

describe("randomId", () => {
	it("uses randomUUID when there is one", () => {
		restore = withCrypto({ randomUUID: () => "from-uuid" });
		expect(randomId()).toBe("from-uuid");
	});

	// crypto.randomUUID exists only over HTTPS or on localhost. Calling it where it
	// is undefined threw synchronously out of send(), before the try — which left
	// the chat busy with a controller nothing would ever clear.
	it("falls back to getRandomValues without randomUUID", () => {
		restore = withCrypto({
			getRandomValues: (array: Uint8Array) => array.fill(0xab),
		});
		expect(randomId()).toBe("ab".repeat(16));
	});

	it("still returns an id with no crypto at all", () => {
		restore = withCrypto(undefined);
		expect(randomId()).toMatch(/^[a-z0-9]+-[a-z0-9]+$/);
	});

	it("does not repeat itself", () => {
		expect(new Set(Array.from({ length: 50 }, randomId)).size).toBe(50);
	});
});

describe("readThreadId", () => {
	/** Just enough sessionStorage to exercise the read-or-mint path. */
	function fakeStorage(initial: Record<string, string> = {}) {
		const store = new Map(Object.entries(initial));
		return {
			getItem: (key: string) => store.get(key) ?? null,
			setItem: (key: string, value: string) => void store.set(key, value),
			removeItem: (key: string) => void store.delete(key),
			read: () => Object.fromEntries(store),
		};
	}

	it("keeps the id this tab is already using", () => {
		const storage = fakeStorage({ [threadKey("u1")]: "existing" });
		vi.stubGlobal("sessionStorage", storage);
		expect(readThreadId("u1")).toBe("existing");
	});

	it("mints and stores one when there is none", () => {
		const storage = fakeStorage();
		vi.stubGlobal("sessionStorage", storage);
		const minted = readThreadId("u1");
		expect(minted).toBeTruthy();
		expect(storage.read()[threadKey("u1")]).toBe(minted);
	});

	// Keyed per user, so signing out and back in as somebody else cannot resume a
	// conversation the agent will refuse to recognise — it keys memory on the
	// authenticated user, not on the thread id alone.
	it("keeps one user's thread out of another's", () => {
		vi.stubGlobal("sessionStorage", fakeStorage({ [threadKey("u1")]: "one" }));
		expect(readThreadId("u2")).not.toBe("one");
	});
});
