"use client";

import { useEffect, useRef } from "react";
import { sql } from "@codemirror/lang-sql";
import { EditorState, Prec } from "@codemirror/state";
import { EditorView, keymap, highlightActiveLine, lineNumbers, placeholder } from "@codemirror/view";
import { bracketMatching } from "@codemirror/language";
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import { panelTheme } from "./yaml-editor-theme";

/**
 * The query console's SQL editor.
 *
 * CodeMirror rather than a textarea, on the same reasoning the values editor
 * gives: a query is a small piece of code, and highlighting keywords, strings and
 * identifiers is what tells `FROM` apart from a column called `from` at a glance.
 * It reuses the values editor's role-token theme, so it follows light and dark
 * with the rest of the panel and carries no palette of its own.
 *
 * Uncontrolled the same way the values editor is: CodeMirror owns the document,
 * and the value is pushed in only when it changes underneath — a table clicked in
 * the tree rewrites the statement — which is what the second effect guards.
 */
export function SqlEditor({
	value,
	onChange,
	onRun,
	placeholder: placeholderText = "SELECT * FROM users ORDER BY created_at DESC LIMIT 20",
	minHeight = "9rem",
}: {
	value: string;
	onChange: (value: string) => void;
	/** Ctrl/Cmd+Enter runs the statement, the shortcut every SQL console shares. */
	onRun: () => void;
	placeholder?: string;
	minHeight?: string;
}) {
	const host = useRef<HTMLDivElement>(null);
	const view = useRef<EditorView | null>(null);
	// Both handlers held in refs so changing them does not tear down the editor.
	// Synced in effects rather than during render, which would be a side effect.
	const notify = useRef(onChange);
	const run = useRef(onRun);
	useEffect(() => {
		notify.current = onChange;
	}, [onChange]);
	useEffect(() => {
		run.current = onRun;
	}, [onRun]);

	useEffect(() => {
		if (!host.current) return;

		const editor = new EditorView({
			parent: host.current,
			state: EditorState.create({
				doc: value,
				extensions: [
					lineNumbers(),
					history(),
					bracketMatching(),
					highlightActiveLine(),
					placeholder(placeholderText),
					sql(),
					// Highest precedence so Mod-Enter runs the statement rather than
					// inserting a newline, whatever the default keymap would do with it.
					Prec.highest(
						keymap.of([
							{
								key: "Mod-Enter",
								run: () => {
									run.current();
									return true;
								},
							},
						]),
					),
					keymap.of([...defaultKeymap, ...historyKeymap]),
					panelTheme,
					EditorView.lineWrapping,
					EditorView.updateListener.of((update) => {
						if (update.docChanged) notify.current(update.state.doc.toString());
					}),
				],
			}),
		});

		view.current = editor;
		return () => {
			editor.destroy();
			view.current = null;
		};
		// Built once; `value` is synchronised by the effect below so typing does not
		// rebuild the editor and lose the cursor.
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, []);

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
			className="overflow-hidden rounded-control border border-border bg-background"
		/>
	);
}
