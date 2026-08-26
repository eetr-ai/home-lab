"use client";

import { useState, useTransition } from "react";
import { Check, RotateCcw } from "lucide-react";
import { Banner, Button, Input } from "@/components/ui";
import { restartWorkload, scaleWorkload } from "@/app/actions/kube";
import { MAX_REPLICAS, parseReplicas } from "@/lib/kube/replicas";
import type { Scale } from "@/lib/api/types";

/**
 * The two things the panel may change about a workload: when its pods last
 * started, and how many there are.
 *
 * Both refuse for an operator outside ADMIN_WRITE_EMAILS, and the refusal is
 * reported here as an inline banner — never a toast, per the UX guidelines, and
 * never a thrown error, which Next redacts in production exactly where the
 * message was the useful part.
 *
 * The restart has no confirmation step. It is reversible, it is what the operator
 * came here to do, and the guidelines reserve inline confirmation for deletes —
 * of which this section has none.
 */
export function WorkloadControls({
	kind,
	namespace,
	name,
	scale,
}: {
	kind: string;
	namespace: string;
	name: string;
	/** Absent for a DaemonSet, which is sized by the nodes it matches. */
	scale?: Scale;
}) {
	const [error, setError] = useState<string | null>(null);
	const [replicas, setReplicas] = useState(String(scale?.replicas ?? 0));
	const [pending, startTransition] = useTransition();

	function run(action: () => Promise<{ ok: boolean; error?: string }>) {
		setError(null);
		startTransition(async () => {
			const result = await action();
			if (!result.ok) setError(result.error ?? "the change could not be made");
		});
	}

	// Null for a cleared or out-of-range field. Number("") is 0, so reading the
	// field directly would offer to scale the workload to zero the moment the
	// operator selected the text to retype it.
	const wanted = parseReplicas(replicas);
	const changed = scale !== undefined && wanted !== null && wanted !== scale.replicas;

	return (
		<div className="flex flex-col gap-3">
			<Banner variant="error" message={error} />

			<div className="flex flex-wrap items-center gap-3">
				<Button
					icon={RotateCcw}
					variant="secondary"
					loading={pending}
					onClick={() => run(() => restartWorkload(kind, namespace, name))}
				>
					Restart
				</Button>

				{scale ? (
					<div className="flex items-center gap-2">
						<label className="text-sm text-muted-foreground" htmlFor="replicas">
							Replicas
						</label>
						<Input
							id="replicas"
							type="number"
							min={0}
							max={MAX_REPLICAS}
							value={replicas}
							onChange={(event) => setReplicas(event.target.value)}
							className="w-20"
						/>
						<Button
							icon={Check}
							variant="secondary"
							disabled={!changed}
							loading={pending}
							onClick={() => {
								if (wanted !== null) void run(() => scaleWorkload(kind, namespace, name, wanted));
							}}
						>
							Apply
						</Button>
						{/* What is running now, which lags the desired count during a
						    rollout — and is the number that says whether it finished. */}
						<span className="text-sm text-muted-foreground">{scale.current} running</span>
					</div>
				) : (
					<span className="text-sm text-muted-foreground">
						Sized by the nodes it matches — there is no replica count to set.
					</span>
				)}
			</div>
		</div>
	);
}
