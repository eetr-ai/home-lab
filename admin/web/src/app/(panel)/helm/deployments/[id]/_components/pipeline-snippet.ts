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
 *
 * `--fail-with-body` because this gets pasted into scripts. Plain `curl` exits 0
 * on a `401` or a `409`, so a copied line would report a deploy that never
 * happened as a success — and the body is still printed, which is where this
 * endpoint says what went wrong. A pipeline that needs to tell a `409` from a
 * `400` reads the status instead; docs/deploying-from-a-pipeline.md has that
 * version.
 */
export function pipelineCurl(origin: string, chartId: string, chartVersion: string): string {
	return [
		`curl -sS --fail-with-body -X PATCH "${pipelineUrl(origin, chartId)}" \\`,
		`  -H "Authorization: Bearer $EETR_API_KEY" \\`,
		`  -H 'Content-Type: application/json' \\`,
		`  -d '${JSON.stringify({ chartVersion })}'`,
	].join("\n");
}
