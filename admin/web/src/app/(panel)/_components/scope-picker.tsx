"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useTransition } from "react";
import { Select } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";

/**
 * The toolbar control that chooses which database or namespace a page is showing.
 *
 * The choice goes in the query string rather than in component state. Several of
 * these pages have no meaning without one — MongoDB's collections, PostgreSQL's
 * extensions — so the selection is part of the address: it can be linked to, it
 * survives a reload, and the back button steps through it.
 *
 * Navigating re-runs the Server Component that fetched the list, which is why the
 * transition's pending flag is worth showing: without it the old rows sit there
 * looking current while the new ones are being fetched.
 */
export function ScopePicker({
	label,
	param,
	options,
	selected,
}: {
	label: string;
	param: string;
	options: string[];
	selected: string;
}) {
	const router = useRouter();
	const searchParams = useSearchParams();
	const [pending, startTransition] = useTransition();

	function choose(next: string) {
		const params = new URLSearchParams(searchParams.toString());
		params.set(param, next);
		startTransition(() => router.push(`?${params.toString()}`));
	}

	return (
		<div className="mb-4 flex items-center gap-2">
			<label className="text-sm text-muted-foreground" htmlFor={`scope-${param}`}>
				{label}
			</label>
			<Select
				id={`scope-${param}`}
				value={selected}
				disabled={pending || options.length === 0}
				onChange={(event) => choose(event.target.value)}
			>
				{options.map((option) => (
					<option key={option} value={option}>
						{option}
					</option>
				))}
			</Select>
			{pending ? <Spinner className="text-muted-foreground" /> : null}
		</div>
	);
}
