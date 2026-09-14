import { redirect } from "next/navigation";
import { auth } from "@/auth";
import { listNamespaces } from "@/app/actions/kube";
import { PanelNav } from "./panel-nav";
import { NamespaceScopeProvider } from "./_components/namespace-scope";
import { readNamespaceScope } from "./_components/namespace-scope-server";
import AgentLauncher from "@/components/agent/AgentLauncher";

/**
 * The signed-in shell: a fixed sidebar, the section content, and the agent.
 *
 * The session check here is defence in depth. `proxy.ts` already refuses an
 * unauthenticated navigation, and the server actions authorize on their own; this
 * is what makes sure the shell never renders for someone who reached it another
 * way.
 *
 * The agent is the third column of the same row, which is what makes its drawer
 * push the page aside rather than cover it. `min-w-0 flex-1` on the content was
 * already there and is what lets it narrow — no other layout change was needed.
 * The launcher renders nothing at all until it has confirmed an agent is
 * configured, so an installation without one pays for none of this.
 *
 * The namespace scope is provided here rather than per section, which is what
 * makes it one choice for the whole panel. The namespace list is fetched once for
 * the same reason: the picker on every cluster page reads this one, instead of
 * each page asking the cluster for the same list. A failed read leaves the list
 * empty and the picker disabled — the pages below report their own failures, and
 * a shell that refused to render because a dropdown could not be filled would
 * take the rest of the panel down with it.
 *
 * The user id rather than the email: it keys which conversation this tab is in,
 * it is written to sessionStorage, and there is no reason for an address to be
 * the thing sitting there. It is the provider's own subject, which is also what
 * the agent keys its memory on — set server-side, in the route handler.
 */
export const dynamic = "force-dynamic";

export default async function PanelLayout({ children }: { children: React.ReactNode }) {
	const session = await auth();
	if (!session?.user || session.error !== undefined) redirect("/");

	const [scope, namespaces] = await Promise.all([readNamespaceScope(), listNamespaces()]);

	return (
		<NamespaceScopeProvider
			scope={scope}
			namespaces={namespaces.ok ? namespaces.data : []}
		>
			<div className="flex min-h-screen bg-background text-foreground">
				<PanelNav email={session.user.email ?? ""} />
				<div className="min-w-0 flex-1">{children}</div>
				<AgentLauncher userKey={session.user.id ?? "anonymous"} />
			</div>
		</NamespaceScopeProvider>
	);
}
