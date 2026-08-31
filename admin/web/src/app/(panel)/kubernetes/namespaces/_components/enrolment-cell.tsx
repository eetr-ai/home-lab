"use client";

import { useState, useTransition } from "react";
import { enrolNamespace, revokeNamespace } from "@/app/actions/kube";
import { Button, InlineDeleteConfirm } from "@/components/ui";
import { enrolmentView } from "@/lib/kube/enrolment";
import type { Namespace } from "@/lib/api/types";

/** The tone of the word, in theme tokens. */
const TONES = {
	ok: "text-success-fg",
	warn: "text-warning-fg",
	muted: "text-muted-foreground",
} as const;

/**
 * A namespace's Helm enrolment, and the one button that fixes it.
 *
 * Setting up and repairing are the same request, so they are the same call here.
 * What changes is the word, because "Set up" and "Repair" answer different
 * questions an operator is asking — and "set up wrongly" is the one worth having
 * a word for at all: bindings an older chart left pointing elsewhere keep failing
 * deploys and look fine from everywhere else in the panel.
 *
 * Which word and which action belongs to which state is decided in
 * lib/kube/enrolment.ts, where it is tested. This renders the answer.
 */
export function EnrolmentCell({
	namespace,
	onError,
}: {
	namespace: Namespace;
	onError: (error: string | null) => void;
}) {
	const [pending, startTransition] = useTransition();
	const [confirming, setConfirming] = useState(false);
	const view = enrolmentView(namespace);

	if (!view) {
		return <span className="text-muted-foreground/60">&mdash;</span>;
	}

	function run(action: () => Promise<{ ok: boolean; error?: string }>) {
		onError(null);
		setConfirming(false);
		startTransition(async () => {
			const result = await action();
			if (!result.ok) onError(result.error ?? "the request failed");
		});
	}

	// Revoking takes the panel's access to a namespace away, and a release
	// installed there stops being readable — so it confirms, the same way deleting
	// a row does. Setting up and repairing add what should be there and do not.
	if (confirming) {
		return (
			<InlineDeleteConfirm
				label="Revoke the panel's access to this namespace?"
				confirmLabel="Revoke"
				busy={pending}
				onConfirm={() => run(() => revokeNamespace(namespace.name))}
				onCancel={() => setConfirming(false)}
			/>
		);
	}

	return (
		<span className="inline-flex items-center gap-2 whitespace-nowrap text-xs">
			<span className={TONES[view.tone]}>{view.label}</span>
			{view.action ? (
				<Button
					variant="secondary"
					className="px-3 py-1 text-xs"
					loading={pending}
					onClick={() =>
						view.action?.danger
							? setConfirming(true)
							: run(() => enrolNamespace(namespace.name))
					}
				>
					{view.action.label}
				</Button>
			) : null}
		</span>
	);
}
