import { describe, expect, it } from "vitest";
import { formatBytes } from "./bytes";

describe("formatBytes", () => {
	const tests: { name: string; bytes: number; want: string }[] = [
		{ name: "zero", bytes: 0, want: "0 B" },
		{ name: "below a kibibyte stays in bytes", bytes: 512, want: "512 B" },
		{ name: "the boundary itself", bytes: 1024, want: "1.0 KiB" },
		{ name: "a fraction of a kibibyte", bytes: 1536, want: "1.5 KiB" },
		{ name: "mebibytes", bytes: 1024 * 1024, want: "1.0 MiB" },
		{ name: "gibibytes", bytes: 3 * 1024 ** 3, want: "3.0 GiB" },
		{ name: "tebibytes", bytes: 1024 ** 4, want: "1.0 TiB" },
		// Rather than reading past the end of the unit table.
		{ name: "beyond the largest unit", bytes: 2048 * 1024 ** 5, want: "2048.0 PiB" },
		{ name: "a negative count is not a size", bytes: -1, want: "—" },
		{ name: "not a number", bytes: Number.NaN, want: "—" },
		{ name: "infinite", bytes: Number.POSITIVE_INFINITY, want: "—" },
	];

	for (const { name, bytes, want } of tests) {
		it(name, () => {
			expect(formatBytes(bytes)).toBe(want);
		});
	}
});
