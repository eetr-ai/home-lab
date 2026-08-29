"use client";

import { useEffect, useState } from "react";
import { listChartVersions } from "@/app/actions/helm";
import type { HelmChartVersion } from "@/lib/api/types";

/** How long to wait after typing stops before contacting the registry. */
const settleMs = 500;

interface Lookup {
	/** The reference these versions are for, so a stale answer is recognisable. */
	reference: string;
	offered: HelmChartVersion[];
	hint: string;
}

const nothing: Lookup = { reference: "", offered: [], hint: "" };

/**
 * The versions a chart reference offers, fetched as the reference is typed.
 *
 * Debounced, because this reaches a registry and a keystroke is not a request. A
 * reference that does not yet look like one is not sent at all — the API would
 * answer 400 for every prefix of what somebody is halfway through typing, and
 * flashing an error under a field being filled in is noise, not feedback.
 *
 * The result carries the reference it belongs to, and anything for a different
 * one is treated as absent. That is what keeps a slow answer for a half-typed
 * reference from populating the picker for a finished one, and it means the
 * effect never has to reset state on its way in.
 *
 * A failure is reported as a hint rather than an error banner: not being able to
 * list versions does not stop anybody declaring one, it just means typing it.
 */
export function useChartVersions(reference: string): {
	offered: HelmChartVersion[];
	hint: string;
} {
	const [lookup, setLookup] = useState<Lookup>(nothing);
	const trimmed = reference.trim();
	const askable = looksLikeAReference(trimmed);

	useEffect(() => {
		if (!askable) return;

		let cancelled = false;
		const timer = setTimeout(async () => {
			const result = await listChartVersions(trimmed);
			if (cancelled) return;

			if (!result.ok) {
				setLookup({
					reference: trimmed,
					offered: [],
					hint: `Could not list versions (${result.error}). Type one instead.`,
				});
				return;
			}
			if (result.data.length === 0) {
				setLookup({
					reference: trimmed,
					offered: [],
					hint: "That repository offers no versions of this chart. Type one instead.",
				});
				return;
			}
			setLookup({
				reference: trimmed,
				offered: result.data,
				hint: `${result.data.length} available`,
			});
		}, settleMs);

		return () => {
			cancelled = true;
			clearTimeout(timer);
		};
	}, [trimmed, askable]);

	if (!askable) return { offered: [], hint: "" };
	if (lookup.reference !== trimmed) return { offered: [], hint: "Looking up versions\u2026" };
	return { offered: lookup.offered, hint: lookup.hint };
}

/** Enough of a reference to be worth asking about: a scheme, a host, and a name. */
function looksLikeAReference(reference: string): boolean {
	if (!/^(oci|https):\/\//.test(reference)) return false;
	const path = reference.replace(/^(oci|https):\/\//, "").replace(/\/+$/, "");
	return path.split("/").filter(Boolean).length >= 2;
}
