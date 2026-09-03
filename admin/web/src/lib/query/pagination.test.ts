import { describe, expect, it } from "vitest";
import {
	advance,
	currentCursor,
	firstPage,
	hasPrevious,
	pageNumber,
	retreat,
} from "./pagination";

describe("browse pagination trail", () => {
	it("starts at the first page with the empty cursor and no way back", () => {
		const trail = firstPage();
		expect(currentCursor(trail)).toBe("");
		expect(hasPrevious(trail)).toBe(false);
		expect(pageNumber(trail)).toBe(1);
	});

	it("advances onto the cursor's page and can then step back", () => {
		const page2 = advance(firstPage(), "cursor-1");
		expect(currentCursor(page2)).toBe("cursor-1");
		expect(hasPrevious(page2)).toBe(true);
		expect(pageNumber(page2)).toBe(2);
	});

	it("retreat is the inverse of advance", () => {
		const page3 = advance(advance(firstPage(), "c1"), "c2");
		expect(currentCursor(page3)).toBe("c2");
		const back = retreat(page3);
		expect(currentCursor(back)).toBe("c1");
		expect(pageNumber(back)).toBe(2);
	});

	it("cannot step back past the first page", () => {
		const trail = firstPage();
		const stillFirst = retreat(trail);
		expect(currentCursor(stillFirst)).toBe("");
		expect(pageNumber(stillFirst)).toBe(1);
		expect(hasPrevious(stillFirst)).toBe(false);
	});
});
