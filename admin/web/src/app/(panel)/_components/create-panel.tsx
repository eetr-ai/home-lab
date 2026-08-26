"use client";

import { useState, type ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { Banner, Button, ConfirmDialog, SidePanel } from "@/components/ui";
import type { ActionResult } from "@/lib/api/result";

/**
 * The side panel every multi-field create form in the panel uses.
 *
 * It owns three things the forms would otherwise each reinvent: the in-flight
 * state, the failure banner, and the unsaved-changes guard. The caller owns the
 * fields and tells this whether they are dirty.
 *
 * The panel never closes itself — `SidePanel` reports the request and this
 * decides — which is exactly what makes interposing that guard possible. The
 * guard's safe choice is the default one, so pressing Enter on it does not
 * discard what you typed.
 *
 * The confirm dialog renders from its own portal as a sibling of the panel rather
 * than inside it: an animated panel is a transformed element and therefore a
 * containing block, so a `fixed` child would resolve against the panel instead of
 * the viewport.
 */
export function CreatePanel({
	open,
	title,
	icon,
	description,
	submitLabel = "Create",
	dirty,
	onClose,
	onSubmit,
	children,
}: {
	open: boolean;
	title: string;
	icon?: LucideIcon;
	description?: ReactNode;
	submitLabel?: string;
	dirty: boolean;
	/** Called once the panel should really go away, so the caller can reset fields. */
	onClose: () => void;
	onSubmit: () => Promise<ActionResult<unknown>>;
	children: ReactNode;
}) {
	const [error, setError] = useState<string | null>(null);
	const [saving, setSaving] = useState(false);
	const [guarding, setGuarding] = useState(false);

	function close() {
		setError(null);
		setGuarding(false);
		onClose();
	}

	function requestClose() {
		if (saving) return;
		if (dirty) {
			setGuarding(true);
			return;
		}
		close();
	}

	async function submit(event: React.FormEvent) {
		event.preventDefault();
		setError(null);
		setSaving(true);
		const result = await onSubmit();
		setSaving(false);
		if (!result.ok) {
			setError(result.error);
			return;
		}
		close();
	}

	return (
		<>
			<SidePanel
				open={open}
				onRequestClose={requestClose}
				title={title}
				icon={icon}
				description={description}
				footer={
					<div className="flex justify-end gap-2">
						<Button type="button" variant="secondary" onClick={requestClose} disabled={saving}>
							Cancel
						</Button>
						<Button type="submit" form="create-panel-form" loading={saving}>
							{submitLabel}
						</Button>
					</div>
				}
			>
				<Banner variant="error" message={error} />
				<form id="create-panel-form" onSubmit={submit} className="space-y-4">
					{children}
				</form>
			</SidePanel>

			<ConfirmDialog
				open={guarding}
				title="Discard this?"
				description="The fields you filled in will be lost."
				confirmLabel="Discard"
				cancelLabel="Keep editing"
				emphasis="cancel"
				onConfirm={close}
				onCancel={() => setGuarding(false)}
			/>
		</>
	);
}
