import { cookies } from "next/headers";
import { NextResponse, type NextRequest } from "next/server";

import { auth } from "@/auth";
import { denyUnlessSignedIn } from "@/app/actions/_auth";
import { connectGoogle } from "@/lib/api/backups";
import { permitsWrite, writeAllowlist } from "@/lib/auth/write-access";
import { type ConnectError, googleRefusal } from "@/lib/backups/connect-outcome";
import { CALLBACK_PATH, FLOW_COOKIE, openFlow, redirectUriFor, sameState } from "@/lib/backups/oauth";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

/**
 * Where Google sends the browser back after the consent screen.
 *
 * A route handler rather than an action, because it is a URL a third party
 * navigates to. That makes it the trust boundary for connecting, the way the
 * action is everywhere else, and it checks four things before the API hears
 * anything: a signed-in operator, one allowed to make changes, a flow cookie this
 * deployment sealed in the last ten minutes, and a state that matches it.
 *
 * Every outcome is a redirect back to the backups page, carrying a code when
 * something went wrong: the operator arrives here mid-flow, and a JSON body would
 * be a dead end. A code and not a sentence, because the page's URL is one anybody
 * can write; the page only ever shows its own sentence for it.
 */
export async function GET(request: NextRequest): Promise<NextResponse> {
	const jar = await cookies();
	const sealed = jar.get(FLOW_COOKIE)?.value;
	// One use: whatever happens next, this flow is over.
	jar.delete({ name: FLOW_COOKIE, path: CALLBACK_PATH });

	const back = (error?: ConnectError) => {
		const target = new URL("/backups", redirectUriFor(process.env.AUTH_URL) ?? request.url);
		if (error) target.searchParams.set("driveError", error);
		else target.searchParams.set("drive", "connected");
		return NextResponse.redirect(target);
	};

	const denied = await denyUnlessSignedIn();
	if (denied) return back("signed_out");
	const session = await auth();
	if (!permitsWrite(writeAllowlist(process.env.ADMIN_WRITE_EMAILS), session?.user?.email)) {
		return back("not_allowed");
	}

	const params = request.nextUrl.searchParams;
	const secret = process.env.AUTH_SECRET ?? "";
	const flow = sealed ? await openFlow(sealed, secret, Date.now()) : null;
	if (!flow || !sameState(flow.state, params.get("state"))) {
		return back("expired");
	}

	// The operator said no, or Google refused before asking them.
	const refusal = params.get("error");
	if (refusal) return back(googleRefusal(refusal));

	const code = params.get("code");
	const redirectUri = redirectUriFor(process.env.AUTH_URL);
	if (!code || !redirectUri) return back("no_code");

	const result = await connectGoogle({ code, codeVerifier: flow.verifier, redirectUri });
	if (result.ok) return back();
	// The reason stays here rather than riding the redirect.
	console.error(`connecting Google Drive failed: ${result.error}`);
	return back("connect_failed");
}
