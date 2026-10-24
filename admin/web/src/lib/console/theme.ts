/**
 * The terminal's colours, from the panel's theme.
 *
 * xterm.js paints its own surface and takes colours as values, not as CSS, so the
 * `--terminal-*` roles in theme.css are read as computed values and handed over.
 * The roles are the only palette: nothing here names a colour. A role that does
 * not resolve is left out, and xterm falls back to its own default for it rather
 * than painting text in an empty string.
 */

/** xterm's theme keys, each mapped to the role that supplies it. */
const ROLES = {
	background: "--terminal-background",
	foreground: "--terminal-foreground",
	cursor: "--terminal-cursor",
	cursorAccent: "--terminal-background",
	selectionBackground: "--terminal-selection",
	black: "--terminal-black",
	red: "--terminal-red",
	green: "--terminal-green",
	yellow: "--terminal-yellow",
	blue: "--terminal-blue",
	magenta: "--terminal-magenta",
	cyan: "--terminal-cyan",
	white: "--terminal-white",
	brightBlack: "--terminal-bright-black",
	brightRed: "--terminal-bright-red",
	brightGreen: "--terminal-bright-green",
	brightYellow: "--terminal-bright-yellow",
	brightBlue: "--terminal-bright-blue",
	brightMagenta: "--terminal-bright-magenta",
	brightCyan: "--terminal-bright-cyan",
	brightWhite: "--terminal-bright-white",
} as const;

export type TerminalTheme = Partial<Record<keyof typeof ROLES, string>>;

/** Build the theme from a role reader — `getComputedStyle(root).getPropertyValue` in the browser. */
export function terminalTheme(read: (role: string) => string): TerminalTheme {
	const theme: TerminalTheme = {};
	for (const [key, role] of Object.entries(ROLES) as [keyof typeof ROLES, string][]) {
		const value = read(role).trim();
		if (value) theme[key] = value;
	}
	return theme;
}
