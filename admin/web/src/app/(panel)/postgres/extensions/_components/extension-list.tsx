"use client";

import { useState, useTransition } from "react";
import { Blocks, Plus } from "lucide-react";
import { installExtension } from "@/app/actions/postgres";
import { Button, Input, Td, Th } from "@/components/ui";
import { Directory } from "../../../_components/directory";
import { ScopePicker } from "../../../_components/scope-picker";
import type { PostgresExtension } from "@/lib/api/types";

/**
 * Extensions have one field, so they get a compact inline add row rather than a
 * side panel — a full overlay to capture one text input costs more screen than it
 * saves. See docs/contributing/ux-guidelines.md.
 *
 * There is no drop: removing an extension can cascade to the objects that depend
 * on it, and the API does not offer it.
 */
export function ExtensionList({
	databases,
	selected,
	extensions,
	loadError,
}: {
	databases: string[];
	selected: string;
	extensions: PostgresExtension[];
	loadError: string | null;
}) {
	const [error, setError] = useState<string | null>(loadError);
	const [name, setName] = useState("");
	const [pending, startTransition] = useTransition();

	function install(event: React.FormEvent) {
		event.preventDefault();
		if (!name.trim()) return;
		setError(null);
		startTransition(async () => {
			const result = await installExtension(selected, name.trim());
			if (!result.ok) {
				setError(result.error);
				return;
			}
			setName("");
		});
	}

	return (
		<>
			<ScopePicker label="Database" param="database" options={databases} selected={selected} />

			{selected ? (
				<form onSubmit={install} className="mb-4 flex flex-wrap items-center gap-2">
					<Input
						aria-label="Extension name"
						value={name}
						onChange={(event) => setName(event.target.value)}
						placeholder="vector"
						autoComplete="off"
						className="w-56"
					/>
					<Button type="submit" icon={Plus} loading={pending} disabled={!name.trim()}>
						Install
					</Button>
				</form>
			) : null}

			<Directory
				error={error}
				isEmpty={extensions.length === 0}
				empty={{
					icon: Blocks,
					title: selected ? "No extensions installed" : "No databases",
					description: selected
						? "pgvector ships with this server as `vector`; `pgcrypto` comes with PostgreSQL itself."
						: "Create a database first.",
				}}
				columns={
					<>
						<Th>Name</Th>
						<Th>Version</Th>
					</>
				}
				rows={extensions.map((extension) => (
					<tr key={extension.name}>
						<Td className="font-medium">{extension.name}</Td>
						<Td className="text-muted-foreground">{extension.version}</Td>
					</tr>
				))}
			/>
		</>
	);
}
