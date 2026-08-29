"use client";

import { useEffect, useRef } from "react";
import { yaml } from "@codemirror/lang-yaml";
import { EditorState } from "@codemirror/state";
import { EditorView, keymap, lineNumbers, highlightActiveLine } from "@codemirror/view";
import { bracketMatching, foldGutter, indentOnInput, indentUnit } from "@codemirror/language";
import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { highlightSelectionMatches, search, searchKeymap } from "@codemirror/search";
import { panelTheme } from "./yaml-editor-theme";

/**
 * A YAML editor for a chart's values.
 *
 * CodeMirror rather than a textarea, and it is worth saying why given this app
 * otherwise adds no UI libraries: a values file is a document with meaningful
 * indentation, and in a bare textarea an indentation mistake is invisible until
 * the API rejects it. Line numbers, folding, bracket matching and a working tab
 * key are the difference between editing a file and typing into a box.
 *
 * Deliberately uncontrolled. CodeMirror owns the document, and re-creating the
 * state on every keystroke would lose the cursor, the undo history, and the fold
 * state. The value is pushed in only when it changes underneath us — a different
 * version selected, or a save that rewrote it — which is what the guard in the
 * second effect is for.
 */
export function YamlEditor({
	value,
	onChange,
	readOnly = false,
	minHeight = "18rem",
}: {
	value: string;
	onChange?: (value: string) => void;
	readOnly?: boolean;
	minHeight?: string;
}) {
	const host = useRef<HTMLDivElement>(null);
	const view = useRef<EditorView | null>(null);
	// Held in a ref so changing the handler does not tear down the editor. Synced
	// in an effect rather than during render: a ref written during render is a
	// side effect, and React is entitled to render twice.
	const notify = useRef(onChange);
	useEffect(() => {
		notify.current = onChange;
	}, [onChange]);

	useEffect(() => {
		if (!host.current) return;

		const editor = new EditorView({
			parent: host.current,
			state: EditorState.create({
				doc: value,
				extensions: [
					lineNumbers(),
					foldGutter(),
					history(),
					// YAML is indentation, so two spaces and a tab key that inserts
					// them rather than moving focus. The accessibility cost of
					// capturing Tab is real; in a code editor the alternative is
					// worse, and Escape then Tab still leaves.
					indentUnit.of("  "),
					indentOnInput(),
					bracketMatching(),
					highlightActiveLine(),
					highlightSelectionMatches(),
					search({ top: true }),
					keymap.of([...defaultKeymap, ...historyKeymap, ...searchKeymap, indentWithTab]),
					yaml(),
					panelTheme,
					EditorView.lineWrapping,
					EditorState.readOnly.of(readOnly),
					EditorView.editable.of(!readOnly),
					EditorView.updateListener.of((update) => {
						if (update.docChanged) notify.current?.(update.state.doc.toString());
					}),
				],
			}),
		});

		view.current = editor;
		return () => {
			editor.destroy();
			view.current = null;
		};
		// Built once. `value` is synchronised by the effect below instead, so that
		// typing does not rebuild the editor and lose the cursor.
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [readOnly]);

	useEffect(() => {
		const editor = view.current;
		if (!editor) return;
		const current = editor.state.doc.toString();
		if (current === value) return;
		editor.dispatch({ changes: { from: 0, to: current.length, insert: value } });
	}, [value]);

	return (
		<div
			ref={host}
			style={{ minHeight }}
			className="overflow-hidden rounded-control border border-border bg-surface"
		/>
	);
}
