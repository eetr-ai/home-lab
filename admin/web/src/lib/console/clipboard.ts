import type { ClipboardSelectionType, IClipboardProvider } from "@xterm/addon-clipboard";

/**
 * The clipboard as the shell may see it: written to, never read.
 *
 * OSC 52 also lets a program ask for the clipboard's contents, and anything
 * printed to the terminal -- a file being `cat`ed, a remote host's output -- could
 * ask. Copying only needs the write half, so reads answer with nothing.
 *
 * Takes the clipboard rather than reaching for navigator's, so the rules are
 * tested rather than discovered in a browser. Undefined is a real value for it:
 * browsers only offer one on a secure origin.
 */
export function writeOnlyClipboard(clipboard: Pick<Clipboard, "writeText"> | undefined): IClipboardProvider {
	return {
		readText: () => "",
		writeText: async (selection: ClipboardSelectionType, text: string) => {
			// "c" is the system clipboard, and empty is what tmux actually sends; the
			// rest are X selections a browser has no equivalent for.
			const target = String(selection);
			if (target !== "c" && target !== "") return;
			// No clipboard on a plain-HTTP origin, and Safari refuses a write that
			// did not come from a click. Either way the copy is lost, not the
			// terminal: a rejection here would land in xterm's parser.
			try {
				await clipboard?.writeText(text);
			} catch {
				// Option-drag and Cmd-C still work.
			}
		},
	};
}
