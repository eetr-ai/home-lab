"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useSyncExternalStore } from "react";
import {
	Boxes,
	Database,
	LayoutDashboard,
	Leaf,
	LogOut,
	PanelLeftClose,
	PanelLeftOpen,
	ServerCog,
	ShipWheel,
} from "lucide-react";
import { endSession } from "@/app/actions/session";
import { ThemeSwitcher } from "@/app/theme-switcher";
import { activeHref, type NavItem } from "@/lib/nav/active";

// Remembered per browser so the choice survives a reload. A convenience, so a
// read that throws (private mode) or comes back empty just leaves the nav open.
//
// Read through useSyncExternalStore rather than an effect: the server has no
// localStorage, so it renders open, and this reconciles to the stored value on
// the client without a setState-in-effect and without a hydration warning.
const collapsedKey = "panel-nav-collapsed";

const collapseListeners = new Set<() => void>();

function readCollapsed(): boolean {
	try {
		return localStorage.getItem(collapsedKey) === "1";
	} catch {
		return false;
	}
}

function subscribeCollapsed(onChange: () => void): () => void {
	collapseListeners.add(onChange);
	return () => collapseListeners.delete(onChange);
}

function writeCollapsed(next: boolean): void {
	try {
		localStorage.setItem(collapsedKey, next ? "1" : "0");
	} catch {
		/* the preference just will not persist */
	}
	collapseListeners.forEach((listener) => listener());
}

const navItems: NavItem[] = [
	{ href: "/overview", label: "Overview", icon: LayoutDashboard },
	{ href: "/postgres", label: "PostgreSQL", icon: Database },
	{ href: "/mongo", label: "MongoDB", icon: Leaf },
	{ href: "/kubernetes", label: "Kubernetes", icon: Boxes },
	// A peer of Kubernetes rather than a tab inside it: it is a different system
	// of record. What is running comes from the cluster; what put it there comes
	// from Helm.
	{ href: "/helm", label: "Helm", icon: ShipWheel },
];

export function PanelNav({ email }: { email: string }) {
	const pathname = usePathname();
	const active = activeHref(
		navItems.map((item) => item.href),
		pathname,
	);

	// Open on the server; reconciles to the remembered choice on the client.
	const collapsed = useSyncExternalStore(subscribeCollapsed, readCollapsed, () => false);

	function toggle() {
		writeCollapsed(!collapsed);
	}

	return (
		<>
			<aside
				// Collapsing to w-0 hands the width back to the content rather than
				// covering it. `inert` while collapsed keeps its links out of the tab
				// order — a zero-width column full of focusable controls is otherwise a
				// tab stop into nothing, the same trap the assistant drawer avoids.
				inert={collapsed}
				className={`sticky top-0 flex h-screen max-h-dvh shrink-0 flex-col self-start overflow-hidden bg-background transition-[width] duration-200 ${
					collapsed ? "w-0 border-r-0" : "w-56 border-r border-border"
				}`}
			>
				<div className="flex shrink-0 items-center justify-between gap-2 border-b border-border p-4">
					<Link
						href="/overview"
						className="flex min-w-0 items-center gap-3 rounded-card outline-none ring-brand focus-visible:ring-2"
					>
						<ServerCog className="h-7 w-7 shrink-0 text-brand" />
						<span className="truncate font-semibold text-foreground">Home Lab</span>
					</Link>
					<button
						type="button"
						onClick={toggle}
						aria-label="Collapse navigation"
						className="shrink-0 rounded-full p-1.5 text-muted-foreground hover:bg-surface-hover hover:text-foreground"
					>
						<PanelLeftClose className="h-4 w-4" />
					</button>
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

			{collapsed ? (
				<button
					type="button"
					onClick={toggle}
					aria-label="Open navigation"
					className="fixed left-3 top-3 z-40 rounded-full border border-border bg-surface p-2 text-muted-foreground shadow-sm hover:bg-surface-hover hover:text-foreground"
				>
					<PanelLeftOpen className="h-5 w-5" />
				</button>
			) : null}
		</>
	);
}
