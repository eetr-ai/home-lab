import { SquareTerminal } from "lucide-react";
import { auth } from "@/auth";
import { Banner } from "@/components/ui/banner";
import { PageHeader } from "@/components/ui/page-header";
import { permitsWrite, writeAllowlist } from "@/lib/auth/write-access";
import { ConsoleLoader } from "./_components/console-loader";

export const dynamic = "force-dynamic";

/**
 * A shell in the lab, as many of them as you like.
 *
 * Each tab is a tmux session in the console pod, which has ssh, kubectl and helm
 * and runs as cluster-admin. That makes it the biggest write the panel offers, so
 * it is gated on write access here for the message and again at the socket for
 * real: the gate in the console pod reads the same session and the same allowlist.
 *
 * The socket is on the panel's own origin by default — the Gateway routes
 * /console/socket to the console — and ADMIN_CONSOLE_SOCKET_URL moves it, which
 * is only for local development, where the gate is a separate port.
 */
export default async function ConsolePage() {
	const session = await auth();
	const allowed = permitsWrite(
		writeAllowlist(process.env.ADMIN_WRITE_EMAILS),
		session?.user?.email,
	);

	return (
		<main className="flex h-screen min-h-0 flex-col p-6">
			<PageHeader
				icon={SquareTerminal}
				title="Console"
				description="A shell in the lab with ssh, kubectl and helm. Every tab is a tmux session that keeps running when you close it; type exit to end one."
			/>
			{allowed ? (
				<ConsoleLoader socketBase={process.env.ADMIN_CONSOLE_SOCKET_URL || "/console/socket"} />
			) : (
				<Banner
					variant="warning"
					message="The console is a cluster-admin shell, so it needs write access to the panel, and this account is read-only."
				/>
			)}
		</main>
	);
}
