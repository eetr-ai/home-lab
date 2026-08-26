import { auth } from "@/auth";
import { denyUnlessSignedIn } from "@/app/actions/_auth";
import { agentBaseUrl } from "@/lib/api/agent";
import { permitsWrite, writeAllowlist } from "@/lib/auth/write-access";

/**
 * Whether there is an agent to talk to.
 *
 * The launcher asks once on mount and renders nothing when the answer is no, which
 * is the default: the agent is opt-in in the chart, so most of the time there is
 * no `AGENT_URL` and no drawer to open. A launcher that opened onto an error would
 * be worse than no launcher.
 *
 * It answers from configuration rather than by reaching the agent. A probe would
 * turn every signed-in page load into a request to another pod, and would say
 * "unavailable" for a rollout that lasts seconds — while the thing the launcher
 * needs to know is whether this installation has an agent at all.
 *
 * It carries the same write check the chat route does, so a read-only operator
 * sees no launcher rather than a button that answers 403. One rule, asserted in
 * two places, because the alternative is a control that exists to be refused.
 */

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function GET(): Promise<Response> {
	const denied = await denyUnlessSignedIn();
	// 200 with `false` rather than 401: the launcher's question is "is there a
	// chat", and for a session that has expired the honest answer is no. It is not
	// an error worth surfacing on a page somebody is about to be redirected off.
	if (denied) return Response.json({ available: false });

	const session = await auth();
	const mayUse = permitsWrite(writeAllowlist(process.env.ADMIN_WRITE_EMAILS), session?.user?.email);

	return Response.json({ available: agentBaseUrl() !== "" && mayUse });
}
