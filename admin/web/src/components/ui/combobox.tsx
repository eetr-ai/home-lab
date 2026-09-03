"use client";

import { useRef, useState, type ReactNode } from "react";
import { ChevronDown } from "lucide-react";
import { Popover } from "./popover";
import { ComboboxList } from "./combobox-list";

export interface ComboboxProps<T> {
	items: readonly T[];
	selected: T | null;
	onSelect: (item: T) => void;
	/** A stable key per item, and the text the search ranks and the trigger shows. */
	toKey: (item: T) => string;
	toText: (item: T) => string;
	/** How a row looks in the list. Defaults to the item's text. */
	renderRow?: (item: T) => ReactNode;
	/** Names the control for the trigger's and the search box's labels. */
	label: string;
	placeholder?: string;
	empty?: ReactNode;
	disabled?: boolean;
	id?: string;
}

/**
 * A searchable select: a trigger that shows the current choice and opens a
 * popover with a search box and a ranked, keyboard-navigable list.
 *
 * It reuses the first-party Popover for the overlay, which portals to the body —
 * so it escapes the `overflow` of whatever card or scroll area it opens from,
 * which a plain absolutely-positioned dropdown could not. The list and the fuzzy
 * ranking behind it are adapted from the octo project's picker.
 */
export function Combobox<T>({
	items,
	selected,
	onSelect,
	toKey,
	toText,
	renderRow,
	label,
	placeholder = "Select…",
	empty,
	disabled = false,
	id,
}: ComboboxProps<T>) {
	const [open, setOpen] = useState(false);
	const trigger = useRef<HTMLButtonElement>(null);

	function choose(item: T) {
		onSelect(item);
		setOpen(false);
		trigger.current?.focus();
	}

	const face = selected === null ? null : toText(selected);

	return (
		<>
			<button
				ref={trigger}
				id={id}
				type="button"
				disabled={disabled}
				onClick={() => setOpen((current) => !current)}
				aria-haspopup="listbox"
				aria-expanded={open}
				aria-label={label}
				className="flex w-full items-center gap-2 rounded-control border border-border bg-background px-3 py-2 text-left text-sm text-foreground hover:bg-surface-hover focus:border-brand focus:outline-none focus:ring-1 focus:ring-brand disabled:opacity-50"
			>
				<span className="min-w-0 flex-1 truncate">
					{face ?? <span className="text-muted-foreground">{placeholder}</span>}
				</span>
				<ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden />
			</button>

			<Popover
				open={open}
				onRequestClose={() => setOpen(false)}
				anchor={trigger}
				title={label}
				width="sm"
			>
				<ComboboxList
					items={items}
					selected={selected}
					onChoose={choose}
					toKey={toKey}
					toText={toText}
					renderRow={renderRow ?? toText}
					label={label}
					empty={empty}
				/>
			</Popover>
		</>
	);
}
