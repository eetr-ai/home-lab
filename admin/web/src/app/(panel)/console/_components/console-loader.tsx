"use client";

import dynamic from "next/dynamic";
import { Spinner } from "@/components/ui";

/**
 * The workspace, loaded in the browser only. xterm.js touches the DOM the moment
 * it is imported and the open tabs live in localStorage, so there is nothing
 * worth rendering on the server — and rendering it there would only mean a
 * hydration mismatch on the tab list.
 */
const ConsoleWorkspace = dynamic(
	() => import("./console-workspace").then((m) => m.ConsoleWorkspace),
	{
		ssr: false,
		loading: () => (
			<div className="flex flex-1 items-center justify-center">
				<Spinner />
			</div>
		),
	},
);

export function ConsoleLoader({ socketBase }: { socketBase: string }) {
	return <ConsoleWorkspace socketBase={socketBase} />;
}
