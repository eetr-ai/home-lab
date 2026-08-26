"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { LayoutDashboard, LogOut, ServerCog } from "lucide-react";
import { endSession } from "@/app/actions/session";
import { ThemeSwitcher } from "@/app/theme-switcher";
import { activeHref, type NavItem } from "@/lib/nav/active";

const navItems: NavItem[] = [{ href: "/overview", label: "Overview", icon: LayoutDashboard }];

export function PanelNav({ email }: { email: string }) {
	const pathname = usePathname();
	const active = activeHref(
		navItems.map((item) => item.href),
		pathname,
	);

	return (
		<aside className="sticky top-0 flex h-screen max-h-dvh w-56 shrink-0 flex-col self-start overflow-hidden border-r border-border bg-background">
			<div className="shrink-0 border-b border-border p-4">
				<Link
					href="/overview"
					className="flex items-center gap-3 rounded-card outline-none ring-brand focus-visible:ring-2"
				>
					<ServerCog className="h-7 w-7 shrink-0 text-brand" />
					<span className="truncate font-semibold text-foreground">Home Lab</span>
				</Link>
			</div>

			<nav className="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto p-4">
				{navItems.map(({ href, label, icon: Icon }) => (
					<Link
						key={href}
						href={href}
						aria-current={href === active ? "page" : undefined}
						className={`flex items-center gap-2 rounded-card px-3 py-2 text-sm font-medium transition-colors ${
							href === active
								? "bg-surface-sunken text-foreground"
								: "text-muted-foreground hover:bg-surface-hover hover:text-foreground"
						}`}
					>
						<Icon className="h-4 w-4" />
						{label}
					</Link>
				))}
			</nav>

			<div className="shrink-0 space-y-2 border-t border-border p-4">
				<div className="flex justify-center pb-1">
					<ThemeSwitcher />
				</div>
				{email ? (
					<p className="truncate px-1 text-center text-xs text-muted-foreground" title={email}>
						{email}
					</p>
				) : null}
				<form action={endSession}>
					<button
						type="submit"
						className="flex w-full items-center gap-2 rounded-full border border-border px-3 py-2 text-sm font-medium text-foreground hover:bg-surface-hover"
					>
						<LogOut className="h-4 w-4" />
						Sign out
					</button>
				</form>
			</div>
		</aside>
	);
}
