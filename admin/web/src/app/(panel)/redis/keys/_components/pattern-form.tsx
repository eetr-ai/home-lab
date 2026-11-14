"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { Search } from "lucide-react";
import { Button, Input } from "@/components/ui";

/**
 * The glob a scan matches keys against, written into the query string.
 *
 * An empty pattern removes the parameter rather than setting it empty, so the
 * unfiltered view has one address, not two.
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
				const trimmed = draft.trim();
				startTransition(() => router.push(trimmed ? `?pattern=${encodeURIComponent(trimmed)}` : "?"));
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
