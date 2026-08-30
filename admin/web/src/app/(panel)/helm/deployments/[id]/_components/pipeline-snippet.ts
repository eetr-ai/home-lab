/**
 * The request a pipeline sends for this deployment, as text somebody can paste.
 *
 * Pure and separate from the card that renders it, because this is the part worth
 * testing: what goes into a CI job is a string, and a string that is subtly wrong
 * fails in somebody else's pipeline at three in the morning rather than here.
 */

/** Where a pipeline addresses this deployment. */
export function pipelineUrl(origin: string, chartId: string): string {
	return `${origin.replace(/\/+$/, "")}/api/v1/charts/${encodeURIComponent(chartId)}`;
}

/**
 * A working `curl` for this deployment.
 *
 * Pre-filled with the version that is declared right now, so what is copied is a
 * request that would succeed rather than a template with a placeholder in it —
 * the version is the field a pipeline replaces anyway, and seeing a real one is
 * how you know the shape of it.
 *
 * The key is the one thing left as a variable. A panel that offered to fill in a
 * credential would be a panel that had one to give.
 */
export function pipelineCurl(origin: string, chartId: string, chartVersion: string): string {
	return [
		`curl -sS -X PATCH "${pipelineUrl(origin, chartId)}" \\`,
		`  -H "Authorization: Bearer $EETR_API_KEY" \\`,
		`  -H 'Content-Type: application/json' \\`,
		`  -d '${JSON.stringify({ chartVersion })}'`,
	].join("\n");
}
