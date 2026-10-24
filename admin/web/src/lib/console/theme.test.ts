import { describe, expect, it } from "vitest";
import { terminalTheme } from "./theme";

describe("terminalTheme", () => {
	it("reads each colour from its role", () => {
		const theme = terminalTheme((role) => ` ${role}-value `);
		expect(theme.background).toBe("--terminal-background-value");
		expect(theme.brightCyan).toBe("--terminal-bright-cyan-value");
		expect(theme.selectionBackground).toBe("--terminal-selection-value");
	});

	it("draws the cursor's text in the background colour", () => {
		const theme = terminalTheme((role) => (role === "--terminal-background" ? "#000" : "#fff"));
		expect(theme.cursorAccent).toBe("#000");
	});

	it("covers all sixteen ANSI colours", () => {
		const theme = terminalTheme(() => "#123456");
		const ansi = Object.keys(theme).filter(
			(key) => !["background", "foreground", "cursor", "cursorAccent", "selectionBackground"].includes(key),
		);
		expect(ansi).toHaveLength(16);
	});

	it("leaves out a role that does not resolve, so xterm keeps its default", () => {
		const theme = terminalTheme((role) => (role === "--terminal-red" ? "" : "#fff"));
		expect(theme).not.toHaveProperty("red");
		expect(theme.green).toBe("#fff");
	});
});
