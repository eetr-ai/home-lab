"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { Search } from "lucide-react";
import { Button, Input } from "@/components/ui";
import { searchHref } from "@/lib/redis/search";

/**
 * The glob a scan matches keys against, written into the query string with a
 * fresh `t` on every submit, so searching again really searches again (see
 * lib/redis/search.ts). An empty pattern leaves the parameter out.
 */
export function PatternForm({ pattern }: { pattern: string }) {
	const router = useRouter();
	const [draft, setDraft] = useState(pattern);
	const [pending, startTransition] = useTransition();

	return (
		<form
			className="mb-4 flex items-center gap-2"
			onSubmit={(event) => {
				event.preventDefault();
				startTransition(() => router.push(searchHref(draft, Date.now())));
			}}
		>
			<Input
				aria-label="Key pattern"
				className="max-w-sm font-mono"
				value={draft}
				onChange={(event) => setDraft(event.target.value)}
				placeholder="lock:*"
				autoComplete="off"
			/>
			<Button type="submit" variant="secondary" icon={Search} loading={pending}>
				Search
			</Button>
		</form>
	);
}
