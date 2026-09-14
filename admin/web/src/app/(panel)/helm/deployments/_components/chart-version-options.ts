/**
 * What the chart version field shows: a list to pick from, or a box to type in.
 *
 * Pure, and out of the component, because the two inputs pull in opposite
 * directions and getting it wrong is silent. A deployment always has a current
 * version, and that version is always among the options — so "are there options"
 * is not the same question as "did a registry answer", and answering the first
 * one turned the upgrade field into a select holding nothing but the version it
 * already had, with the hint underneath telling the operator to type a different
 * one and nowhere to type it.
 *
 * `offered` decides. `options` is only what the picker lists once there is one.
 */
export interface VersionField {
	/** A picker rather than a text field. False until a registry has answered. */
	picker: boolean;
	/**
	 * What the picker lists: what the registry offers, plus the current value when
	 * the registry does not offer it. A `select` whose value is not one of its
	 * options renders as the first one, so a chart that has dropped the version a
	 * release runs on would show — and then save — a different version than the
	 * one it was asked about.
	 */
	options: string[];
}

export function versionField(offered: string[], value: string): VersionField {
	if (offered.length === 0) return { picker: false, options: [] };
	return {
		picker: true,
		options: value !== "" && !offered.includes(value) ? [value, ...offered] : offered,
	};
}
