import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { EditorView } from "@codemirror/view";
import { tags } from "@lezer/highlight";
import type { Extension } from "@codemirror/state";

/**
 * The editor's appearance, built entirely from the panel's own role tokens.
 *
 * Every colour here is a `var(--…)` defined in theme.css rather than a literal,
 * which is what makes the editor follow light and dark without a second palette
 * living in TypeScript — and what keeps it honest with the rest of the panel when
 * a token changes. CodeMirror takes CSS values verbatim, so this costs nothing.
 */
const appearance = EditorView.theme({
	"&": {
		backgroundColor: "var(--surface)",
		color: "var(--foreground)",
		fontSize: "0.8125rem",
	},
	"&.cm-focused": { outline: "none" },
	".cm-scroller": {
		fontFamily: "var(--font-mono)",
		lineHeight: "1.6",
	},
	".cm-content": { padding: "0.75rem 0" },
	".cm-gutters": {
		backgroundColor: "var(--surface-sunken)",
		color: "var(--editor-gutter)",
		border: "none",
		borderRight: "1px solid var(--border)",
	},
	".cm-activeLine": { backgroundColor: "var(--editor-active-line)" },
	".cm-activeLineGutter": { backgroundColor: "transparent" },
	".cm-cursor, .cm-dropCursor": { borderLeftColor: "var(--foreground)" },
	// The selection is painted by CodeMirror's own layer once the view is
	// focused, and by the browser otherwise; both need saying or a selection
	// disappears the moment the editor loses focus.
	"&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": {
		backgroundColor: "var(--editor-selection)",
	},
	".cm-selectionMatch": { backgroundColor: "var(--editor-selection)" },
	".cm-matchingBracket, &.cm-focused .cm-matchingBracket": {
		backgroundColor: "var(--editor-selection)",
		outline: "none",
	},
	".cm-panels": {
		backgroundColor: "var(--surface-sunken)",
		color: "var(--foreground)",
	},
	".cm-panels input": {
		backgroundColor: "var(--surface)",
		color: "var(--foreground)",
		border: "1px solid var(--border)",
		borderRadius: "var(--radius-control)",
		padding: "0.125rem 0.375rem",
	},
	".cm-tooltip": {
		backgroundColor: "var(--surface)",
		border: "1px solid var(--border)",
		borderRadius: "var(--radius-control)",
	},
});

/**
 * YAML's tokens, mapped onto the same three or four colours the rest of the panel
 * uses.
 *
 * Deliberately sparse. A values file is read to answer "what did I set", so keys
 * and their values want to be distinguishable and nothing else wants to compete;
 * a full rainbow theme would be louder than every other surface here.
 */
const highlighting = HighlightStyle.define([
	{ tag: [tags.propertyName, tags.definition(tags.propertyName)], color: "var(--editor-key)" },
	{ tag: [tags.string, tags.special(tags.string)], color: "var(--editor-string)" },
	{ tag: [tags.number, tags.bool, tags.null], color: "var(--editor-number)" },
	{ tag: tags.comment, color: "var(--editor-comment)", fontStyle: "italic" },
	{ tag: [tags.punctuation, tags.separator, tags.meta], color: "var(--editor-punctuation)" },
	{ tag: tags.invalid, color: "var(--danger-fg)" },
]);

export const panelTheme: Extension = [appearance, syntaxHighlighting(highlighting)];
