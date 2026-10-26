/**
 * The cron lines the schedule form offers, and how a schedule's line reads back.
 *
 * Presets rather than a cron builder: almost every backup schedule is one of
 * these, and the custom field is there for the rest. The API server is what
 * checks a line — the form never parses one — so there is exactly one parser
 * deciding what is valid, and it is the one that will run it.
 */

export interface CronPreset {
	cron: string;
	label: string;
}

export const CRON_PRESETS: CronPreset[] = [
	{ cron: "0 3 * * *", label: "Every night at 03:00" },
	{ cron: "0 */6 * * *", label: "Every six hours" },
	{ cron: "0 * * * *", label: "Every hour" },
	{ cron: "0 3 * * 0", label: "Every Sunday at 03:00" },
	{ cron: "0 3 1 * *", label: "On the 1st of every month at 03:00" },
];

/** The value the preset select uses for "something else". */
export const CUSTOM_CRON = "custom";

/** Which preset a line is, or CUSTOM_CRON. Whitespace does not count. */
export function presetFor(cron: string): string {
	const normalized = normalizeCron(cron);
	return CRON_PRESETS.some((preset) => preset.cron === normalized) ? normalized : CUSTOM_CRON;
}

/**
 * A schedule's line in words when it is a preset, and in a sentence that says it
 * is cron otherwise. Not a general cron-to-English translator, which would be a
 * second parser to disagree with the real one.
 */
export function describeCron(cron: string, timeZone?: string): string {
	const normalized = normalizeCron(cron);
	const preset = CRON_PRESETS.find((one) => one.cron === normalized);
	const when = preset ? preset.label : `On the cron line ${normalized}`;
	return timeZone ? `${when} (${timeZone})` : `${when} (cluster time)`;
}

function normalizeCron(cron: string): string {
	return cron.trim().split(/\s+/).join(" ");
}
