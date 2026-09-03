"use client";

import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { Check, Search } from "lucide-react";
import { filterRanked } from "@/lib/search/rank";

/**
 * The searchable list inside a {@link Combobox}'s popover: a fixed search box and
 * a ranked, keyboard-navigable listbox below it. The ranking is the shared
 * subsequence scorer in `lib/search/rank`. Adapted from the octo project's picker
 * panel; the palette is this app's role tokens rather than literal colours.
 */
export function ComboboxList<T>({
	items,
	selected,
	onChoose,
	toKey,
	toText,
	renderRow,
	label,
	empty,
}: {
	items: readonly T[];
	selected: T | null;
	onChoose: (item: T) => void;
	toKey: (item: T) => string;
	toText: (item: T) => string;
	renderRow: (item: T) => ReactNode;
	label: string;
	empty?: ReactNode;
}) {
	const [query, setQuery] = useState("");
	const [cursor, setCursor] = useState(0);
	const input = useRef<HTMLInputElement>(null);
	const listbox = useRef<HTMLDivElement>(null);
	const prefix = useId();

	const matches = useMemo(() => filterRanked(items, query, toText), [items, query, toText]);
	// Clamp on read so narrowing the list never leaves the cursor past the end.
	const active = Math.min(cursor, Math.max(matches.length - 1, 0));
	const selectedKey = selected === null ? null : toKey(selected);

	useEffect(() => {
		input.current?.focus();
	}, []);
	useEffect(() => {
		const option = listbox.current?.querySelector<HTMLElement>('[data-active="true"]');
		option?.scrollIntoView?.({ block: "nearest" });
	}, [active, matches.length]);

	function onKeyDown(event: KeyboardEvent) {
		const count = matches.length;
		switch (event.key) {
			case "ArrowDown":
				event.preventDefault();
				setCursor(count ? (active + 1) % count : 0);
				break;
			case "ArrowUp":
				event.preventDefault();
				setCursor(count ? (active - 1 + count) % count : 0);
				break;
			case "Home":
				event.preventDefault();
				setCursor(0);
				break;
			case "End":
				event.preventDefault();
				setCursor(Math.max(count - 1, 0));
				break;
			case "Enter":
				event.preventDefault();
				if (matches[active]) onChoose(matches[active]);
				break;
		}
	}

	return (
		<div className="flex max-h-[min(60vh,24rem)] flex-col">
			<div className="flex items-center gap-2 border-b border-border px-3 py-2">
				<Search className="h-3.5 w-3.5 shrink-0 text-muted-foreground" aria-hidden />
				<input
					ref={input}
					data-autofocus
					role="combobox"
					aria-expanded
					aria-controls={`${prefix}-listbox`}
					aria-activedescendant={matches[active] ? `${prefix}-option-${active}` : undefined}
					value={query}
					onChange={(event) => setQuery(event.target.value)}
					onKeyDown={onKeyDown}
					placeholder={`Search ${label.toLowerCase()}`}
					aria-label={`Search ${label.toLowerCase()}`}
					autoComplete="off"
					className="min-w-0 flex-1 bg-transparent text-sm text-foreground outline-none placeholder:text-muted-foreground"
				/>
			</div>

			<div
				ref={listbox}
				id={`${prefix}-listbox`}
				role="listbox"
				aria-label={label}
				className="min-h-0 flex-1 overflow-y-auto py-1"
			>
				{matches.length === 0 ? (
					<p className="px-3 py-4 text-xs text-muted-foreground">
						{items.length === 0 ? (empty ?? "Nothing to choose from.") : "Nothing matches what you typed."}
					</p>
				) : (
					matches.map((item, index) => {
						const key = toKey(item);
						const isSelected = selectedKey === key;
						return (
							<div
								key={key}
								id={`${prefix}-option-${index}`}
								role="option"
								aria-selected={isSelected}
								data-active={index === active}
								// preventDefault keeps focus on the search input, so the keyboard
								// stays live after a mouse touch.
								onPointerDown={(event) => event.preventDefault()}
								onClick={() => onChoose(item)}
								onMouseMove={() => setCursor(index)}
								className={`flex cursor-pointer items-center gap-2 px-3 py-1.5 text-sm ${
									index === active ? "bg-surface-hover" : ""
								} ${isSelected ? "text-brand" : "text-foreground"}`}
							>
								<span className="min-w-0 flex-1 truncate">{renderRow(item)}</span>
								{isSelected ? <Check className="h-3.5 w-3.5 shrink-0 text-brand" aria-hidden /> : null}
							</div>
						);
					})
				)}
			</div>
		</div>
	);
}
