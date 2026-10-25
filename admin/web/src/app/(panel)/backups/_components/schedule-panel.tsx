"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { DatabaseBackup } from "lucide-react";
import { createSchedule, updateSchedule } from "@/app/actions/backups";
import { Checkbox, FormField, Input, Select } from "@/components/ui";
import { CreatePanel } from "../../_components/create-panel";
import { CRON_PRESETS, CUSTOM_CRON, presetFor } from "@/lib/backups/cron";
import { emptyForm, formFrom, MAX_KEEP, toRequest, type ScheduleForm } from "@/lib/backups/form";
import type { BackupDatabase, BackupSchedule, GoogleStatus } from "@/lib/api/types";
import { DatabasePicker } from "./database-picker";
import { FolderField } from "./folder-field";

/** The browser's own zone, which is almost always the one somebody means by 03:00. */
function browserZone(): string {
	try {
		return Intl.DateTimeFormat().resolvedOptions().timeZone ?? "";
	} catch {
		return "";
	}
}

/**
 * Creating a schedule, or editing one when `schedule` is given.
 *
 * The form lives in lib/backups/form.ts, which is where it is checked and turned
 * into a request; this is only the fields.
 */
export function SchedulePanel({
	open,
	schedule,
	databases,
	status,
	onClose,
}: {
	open: boolean;
	schedule?: BackupSchedule;
	databases: BackupDatabase[];
	status: GoogleStatus | null;
	onClose: () => void;
}) {
	const initial = () => (schedule ? formFrom(schedule) : emptyForm(browserZone()));
	const [form, setForm] = useState<ScheduleForm>(initial);
	const [custom, setCustom] = useState(presetFor(form.cron) === CUSTOM_CRON);
	const [seededFrom, setSeededFrom] = useState(schedule);
	const router = useRouter();

	// The panel stays mounted for the life of the page; a schedule refreshed while
	// it sits closed must reseed it, and an open one is somebody's edit.
	if (!open && seededFrom !== schedule) {
		setSeededFrom(schedule);
		setForm(initial());
	}

	const set = <K extends keyof ScheduleForm>(key: K, value: ScheduleForm[K]) =>
		setForm((current) => ({ ...current, [key]: value }));

	function close() {
		setForm(initial());
		setCustom(presetFor(initial().cron) === CUSTOM_CRON);
		onClose();
	}

	return (
		<CreatePanel
			open={open}
			title={schedule ? `Edit ${schedule.name}` : "New backup schedule"}
			icon={DatabaseBackup}
			submitLabel={schedule ? "Save" : "Create"}
			description="Each run dumps the chosen databases, uploads them to the folder, and keeps the newest backups of each."
			dirty={JSON.stringify(form) !== JSON.stringify(initial())}
			onClose={close}
			onSubmit={async () => {
				const built = toRequest(form);
				if (!built.ok) return built;
				const result = schedule
					? await updateSchedule(schedule.name, built.request)
					: await createSchedule(built.request);
				if (result.ok && !schedule) router.push(`/backups/${encodeURIComponent(result.data.name)}`);
				return result;
			}}
		>
			<FormField label="Name" htmlFor="schedule-name">
				<Input
					id="schedule-name"
					value={form.name}
					onChange={(event) => set("name", event.target.value)}
					placeholder="nightly"
					disabled={Boolean(schedule)}
					autoComplete="off"
					spellCheck={false}
					required
				/>
			</FormField>

			<FormField label="When" htmlFor="schedule-when">
				<Select
					id="schedule-when"
					value={custom ? CUSTOM_CRON : presetFor(form.cron)}
					onChange={(event) => {
						const value = event.target.value;
						setCustom(value === CUSTOM_CRON);
						if (value !== CUSTOM_CRON) set("cron", value);
					}}
				>
					{CRON_PRESETS.map((preset) => (
						<option key={preset.cron} value={preset.cron}>
							{preset.label}
						</option>
					))}
					<option value={CUSTOM_CRON}>A cron line of my own…</option>
				</Select>
				{custom ? (
					<Input
						className="mt-2 font-mono"
						aria-label="Cron line"
						value={form.cron}
						onChange={(event) => set("cron", event.target.value)}
						placeholder="15 2 * * 1-5"
						spellCheck={false}
					/>
				) : null}
			</FormField>

			<FormField label="Time zone" htmlFor="schedule-zone">
				<Input
					id="schedule-zone"
					value={form.timeZone}
					onChange={(event) => set("timeZone", event.target.value)}
					placeholder="UTC"
					spellCheck={false}
				/>
			</FormField>

			<DatabasePicker
				databases={databases}
				selected={form.databases}
				onChange={(selected) => set("databases", selected)}
			/>

			<FolderField status={status} folder={form.folder} onChange={(folder) => set("folder", folder)} />

			<FormField label="Backups to keep of each database" htmlFor="schedule-keep">
				<Input
					id="schedule-keep"
					type="number"
					min={1}
					max={MAX_KEEP}
					value={form.keep}
					onChange={(event) => set("keep", event.target.value)}
				/>
			</FormField>

			<Checkbox
				label="Suspended"
				hint="Keeps the schedule without running it. Run now still works."
				checked={form.suspended}
				onChange={(checked) => set("suspended", checked)}
			/>
		</CreatePanel>
	);
}
