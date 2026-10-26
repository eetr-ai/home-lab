import { denyUnlessSignedIn } from "@/app/actions/_auth";
import { adminApiBaseUrl, bearerToken } from "@/lib/api/config";

/**
 * A backup schedule's event stream, proxied to the admin API.
 *
 * A route handler rather than a server action for the reason the Helm job
 * stream gives: an action's answer is one payload, and this one has no last
 * event — it follows the schedule for as long as its page is open.
 *
 * `signal: req.signal` matters as much here as there. The browser leaving cancels
 * this fetch, which drops the connection to the admin API, which ends the loop
 * reading the cluster for this tab.
 */

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

/** Nginx-style: the client hung up before a response was finished. */
const CLIENT_CLOSED_REQUEST = 499;

export async function GET(req: Request): Promise<Response> {
	const denied = await denyUnlessSignedIn();
	if (denied) return Response.json({ error: denied.error }, { status: 401 });

	const url = new URL(req.url);
	const name = url.searchParams.get("name");
	if (!name) {
		return Response.json({ error: "a schedule is required" }, { status: 400 });
	}

	const base = adminApiBaseUrl();
	if (!base) {
		return Response.json(
			{ error: "the admin API is not configured (ADMIN_API_URL is unset)" },
			{ status: 503 },
		);
	}

	const credential = await bearerToken();
	if ("error" in credential) {
		return Response.json({ error: credential.error }, { status: 401 });
	}

	const upstream = new URL(
		`${base}/api/backups/schedules/${encodeURIComponent(name)}/events`,
	);

	try {
		const response = await fetch(upstream, {
			headers: { Authorization: `Bearer ${credential.token}`, Accept: "text/event-stream" },
			signal: req.signal,
			cache: "no-store",
		});

		if (!response.ok || !response.body) {
			// The API's own prose, not a bare status: it says whether the schedule
			// is gone, or whether the ServiceAccount may not read it at all.
			const detail = await response.text();
			return new Response(detail || `the admin API answered ${response.status}`, {
				status: response.status,
				headers: { "Content-Type": "text/plain; charset=utf-8" },
			});
		}

		return new Response(response.body, {
			status: 200,
			headers: {
				"Content-Type": "text/event-stream",
				"Cache-Control": "no-cache, no-transform",
				Connection: "keep-alive",
				// Belt and braces with the API's own header: nothing between here and
				// the browser may buffer events into batches. A progress stream that
				// arrives all at once at the end is not a progress stream.
				"X-Accel-Buffering": "no",
			},
		});
	} catch (err) {
		// An abort is how a followed stream normally ends — the operator closed the
		// page, or the panel's own pods were replaced by the upgrade it was
		// watching — so it is not an error to report anywhere.
		if (err instanceof Error && err.name === "AbortError") {
			return new Response(null, { status: CLIENT_CLOSED_REQUEST });
		}
		return new Response(`the admin API is unreachable: ${(err as Error).message}`, {
			status: 502,
			headers: { "Content-Type": "text/plain; charset=utf-8" },
		});
	}
}
