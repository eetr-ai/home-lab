import { redirect } from "next/navigation";
import { auth } from "@/auth";
import { PanelNav } from "./panel-nav";

/**
 * The signed-in shell: a fixed sidebar beside the section content.
 *
 * The session check here is defence in depth. `proxy.ts` already refuses an
 * unauthenticated navigation, and the server actions authorize on their own; this
 * is what makes sure the shell never renders for someone who reached it another
 * way.
 */
export const dynamic = "force-dynamic";

export default async function PanelLayout({ children }: { children: React.ReactNode }) {
	const session = await auth();
	if (!session?.user || session.error !== undefined) redirect("/");

	return (
		<div className="flex min-h-screen bg-background text-foreground">
			<PanelNav email={session.user.email ?? ""} />
			<div className="min-w-0 flex-1">{children}</div>
		</div>
	);
}
