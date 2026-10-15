"use client";

import { useCallback, useEffect, useRef, useState } from "react";
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

export interface ChartVersions {
	offered: HelmChartVersion[];
	hint: string;
	/** True while the registry is being asked. */
	loading: boolean;
	/** Whether the reference is complete enough to ask about at all. */
	askable: boolean;
	/** Ask now, regardless of the debounce. What the refresh button calls. */
	refresh: () => void;
}

/**
 * The versions a chart reference offers.
 *
 * Two ways in, because the two places that need this want opposite things. The
 * declare form has a reference being typed and `follow` on, so the picker fills
 * itself in as soon as the reference looks complete. The values card has a
 * reference that was settled weeks ago and wants nothing on load: opening a
 * deployment should not reach a registry, so it leaves `follow` off and the
 * operator asks by pressing refresh.
 *
 * Debounced in either case, because this reaches a registry and a keystroke is
 * not a request. A reference that does not yet look like one is not sent at all —
 * the API would answer 400 for every prefix of what somebody is halfway through
 * typing, and flashing an error under a field being filled in is noise, not
 * feedback.
 *
 * The result carries the reference it belongs to, and anything for a different
 * one is treated as absent. That is what keeps a slow answer for a half-typed
 * reference from populating the picker for a finished one, and it means the
 * effect never has to reset state on its way in.
 *
 * A failure is reported as a hint rather than an error banner: not being able to
 * list versions does not stop anybody declaring one, it just means typing it.
 */
export function useChartVersions(reference: string, follow = false): ChartVersions {
	const [lookup, setLookup] = useState<Lookup>(nothing);
	const [loading, setLoading] = useState(false);
	const trimmed = reference.trim();
	const askable = looksLikeAReference(trimmed);

	// Which request the answer being awaited belongs to. A refresh pressed while
	// an earlier one is still in flight must not have the earlier one's result
	// land on top of it, and comparing references is not enough — pressing
	// refresh twice for the same reference is exactly the case where they match.
	const latest = useRef(0);

	const ask = useCallback(async (target: string) => {
		const id = ++latest.current;
		setLoading(true);
		const result = await listChartVersions(target);
		if (id !== latest.current) return;
		setLoading(false);

		if (!result.ok) {
			setLookup({
				reference: target,
				offered: [],
				hint: `Could not list versions (${result.error}). Type one instead.`,
			});
			return;
		}
		if (result.data.length === 0) {
			setLookup({
				reference: target,
				offered: [],
				hint: "That repository offers no versions of this chart. Type one instead.",
			});
			return;
		}
		setLookup({
			reference: target,
			offered: result.data,
			hint: `${result.data.length} available`,
		});
	}, []);

	useEffect(() => {
		if (!follow || !askable) return;

		const timer = setTimeout(() => void ask(trimmed), settleMs);
		return () => clearTimeout(timer);
	}, [trimmed, askable, follow, ask]);

	const refresh = useCallback(() => {
		if (askable) void ask(trimmed);
	}, [askable, trimmed, ask]);

	if (!askable) return { offered: [], hint: "", loading: false, askable, refresh };
	if (loading || (follow && lookup.reference !== trimmed)) {
		return { offered: [], hint: "Looking up versions…", loading: true, askable, refresh };
	}
	if (lookup.reference !== trimmed) {
		return { offered: [], hint: "", loading: false, askable, refresh };
	}
	return { offered: lookup.offered, hint: lookup.hint, loading: false, askable, refresh };
}

/** Enough of a reference to be worth asking about: a scheme, a host, and a name. */
function looksLikeAReference(reference: string): boolean {
	if (!/^(oci|https):\/\//.test(reference)) return false;
	const path = reference.replace(/^(oci|https):\/\//, "").replace(/\/+$/, "");
	return path.split("/").filter(Boolean).length >= 2;
}
