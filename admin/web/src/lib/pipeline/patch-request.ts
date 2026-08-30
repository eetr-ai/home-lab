/**
 * The body a pipeline sends to `PATCH /api/v1/charts/{chartId}`, and what it
 * becomes on the way to the admin API.
 *
 * Its own vocabulary on purpose. A pipeline thinks in charts and versions, and
 * `{chartVersion, valueOverrides}` says what it is doing; the admin API's
 * `{version, values}` is the older, shorter spelling of the same thing from
 * inside a service that only ever talks about Helm. The translation is one
 * function so the two names meet in exactly one place.
 *
 * Pure, and no `server-only`: it reads nothing and reaches nowhere, which is what
 * lets the shape a CI job depends on be tested without a server.
 */

/** What this endpoint accepts. */
export interface ChartPatch {
	chartVersion: string;
	valueOverrides?: Record<string, unknown>;
}

/** What the admin API's `PUT /api/helm/deployments/{id}` accepts. */
export interface PipelineRequest {
	version: string;
	values?: Record<string, unknown>;
}

/** A parsed body, or the sentence to answer 400 with. */
export type ParsedPatch = { request: PipelineRequest } | { error: string };

/**
 * Read a decoded JSON body into an admin-API request.
 *
 * The version is not validated beyond being a non-empty string. The API's own
 * `validateVersion` refuses ranges and `latest`, and a second implementation here
 * would be a second thing to keep in agreement — one that fails differently and
 * only sometimes.
 *
 * `rollbackOnFailure` is deliberately not accepted. It is off by default because
 * a rollback lands a failed deploy back on `deployed` at the old version, which
 * is the one outcome a pipeline is most likely to misread as success; turning
 * that on is a decision for whoever owns the release, not a field in a CI script.
 */
export function parsePatch(body: unknown): ParsedPatch {
	// `res.json()` resolves to whatever decoded — null, an array, a number — so
	// this is checked before anything is read off it. Nothing here may throw.
	if (body === null || typeof body !== "object" || Array.isArray(body)) {
		return { error: "the body must be a JSON object" };
	}

	const { chartVersion, valueOverrides } = body as ChartPatch;

	if (typeof chartVersion !== "string" || chartVersion.trim() === "") {
		return { error: "chartVersion is required and must be a non-empty string" };
	}

	if (valueOverrides === undefined || valueOverrides === null) {
		// Omitted rather than sent empty, and the difference is not cosmetic: the
		// API carries the previous values forward byte for byte when there are no
		// overrides, comments included, and regenerates the document when there are.
		return { request: { version: chartVersion } };
	}

	if (typeof valueOverrides !== "object" || Array.isArray(valueOverrides)) {
		return { error: "valueOverrides must be a JSON object" };
	}

	return { request: { version: chartVersion, values: valueOverrides } };
}
