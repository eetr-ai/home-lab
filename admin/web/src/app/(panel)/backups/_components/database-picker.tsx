"use client";

import { Checkbox, Label } from "@/components/ui";
import { databaseKey } from "@/lib/backups/form";
import type { BackupDatabase } from "@/lib/api/types";

const ENGINES = [
	{ engine: "postgres", label: "PostgreSQL" },
	{ engine: "mongo", label: "MongoDB" },
] as const;

/**
 * The databases a schedule backs up, grouped by server.
 *
 * A database the schedule already names but the server no longer lists is still
 * shown, checked, so editing a schedule never drops one silently.
 */
export function DatabasePicker({
	databases,
	selected,
	onChange,
}: {
	databases: BackupDatabase[];
	selected: string[];
	onChange: (selected: string[]) => void;
}) {
	const offered = new Set(databases.map(databaseKey));
	const all = [
		...databases,
		...selected
			.filter((key) => !offered.has(key))
			.map((key) => ({ engine: key.split("/")[0], name: key.slice(key.indexOf("/") + 1) }) as BackupDatabase),
	];

	function toggle(key: string, checked: boolean) {
		onChange(checked ? [...selected, key] : selected.filter((one) => one !== key));
	}

	return (
		<fieldset className="flex flex-col gap-3">
			<Label>Databases</Label>
			{all.length === 0 ? (
				<p className="text-sm text-muted-foreground">Neither server listed any databases.</p>
			) : null}
			{ENGINES.map(({ engine, label }) => {
				const group = all.filter((database) => database.engine === engine);
				if (group.length === 0) return null;
				return (
					<div key={engine} className="flex flex-col gap-1">
						<span className="text-xs font-medium uppercase text-muted-foreground">{label}</span>
						{group.map((database) => {
							const key = databaseKey(database);
							return (
								<Checkbox
									key={key}
									label={database.name}
									checked={selected.includes(key)}
									onChange={(checked) => toggle(key, checked)}
								/>
							);
						})}
					</div>
				);
			})}
			{selected.some((key) => key.startsWith("postgres/")) ? (
				<p className="text-xs text-muted-foreground">
					PostgreSQL runs also keep the server&apos;s roles, as postgres/@globals.
				</p>
			) : null}
		</fieldset>
	);
}
