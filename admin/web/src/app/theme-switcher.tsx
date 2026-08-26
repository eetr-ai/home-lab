"use client";

import { useEffect, useSyncExternalStore } from "react";
import { Sun, Moon, Monitor } from "lucide-react";

type Theme = "light" | "dark" | "system";

const STORAGE_KEY = "theme";

/** Same-tab notification. `storage` events only reach *other* tabs. */
const CHANGED_EVENT = "home-lab:theme-changed";

const options: { value: Theme; label: string; icon: typeof Sun }[] = [
	{ value: "light", label: "Light", icon: Sun },
	{ value: "system", label: "System", icon: Monitor },
	{ value: "dark", label: "Dark", icon: Moon },
];

/** The stored preference, or "system" when there is none or it is unrecognized. */
function readTheme(): Theme {
	try {
		const stored = localStorage.getItem(STORAGE_KEY);
		return stored === "light" || stored === "dark" || stored === "system" ? stored : "system";
	} catch {
		// Storage can throw outright when the browser blocks site data.
		return "system";
	}
}

function subscribe(onChange: () => void): () => void {
	window.addEventListener("storage", onChange);
	window.addEventListener(CHANGED_EVENT, onChange);
	return () => {
		window.removeEventListener("storage", onChange);
		window.removeEventListener(CHANGED_EVENT, onChange);
	};
}

// Resolve the preference to a concrete light/dark class on <html>. Mirrors the
// inline pre-paint script in layout.tsx so they stay in sync.
function applyTheme(theme: Theme) {
	const dark =
		theme === "dark" ||
		(theme === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches);
	const root = document.documentElement.classList;
	root.toggle("dark", dark);
	root.toggle("light", !dark);
}

export function ThemeSwitcher() {
	// The preference lives in localStorage, which the server cannot read, so this
	// is a subscription to an external store rather than state synced by an
	// effect. React renders the server snapshot ("system") through hydration and
	// swaps in the real value afterwards, with no mismatch and no `mounted` flag.
	const theme = useSyncExternalStore(subscribe, readTheme, () => "system" as Theme);

	// Keep following the OS while "system" is selected.
	useEffect(() => {
		if (theme !== "system") return;
		const mql = window.matchMedia("(prefers-color-scheme: dark)");
		const onChange = () => applyTheme("system");
		mql.addEventListener("change", onChange);
		return () => mql.removeEventListener("change", onChange);
	}, [theme]);

	function choose(next: Theme) {
		try {
			localStorage.setItem(STORAGE_KEY, next);
		} catch {
			// Blocked storage costs the preference its persistence, not the click.
		}
		applyTheme(next);
		window.dispatchEvent(new Event(CHANGED_EVENT));
	}

	return (
		<div
			role="group"
			aria-label="Theme"
			className="inline-flex items-center gap-0.5 rounded-full border border-border p-0.5"
		>
			{options.map(({ value, label, icon: Icon }) => {
				const isActive = theme === value;
				return (
					<button
						key={value}
						type="button"
						onClick={() => choose(value)}
						aria-label={label}
						aria-pressed={isActive}
						title={label}
						className={`inline-flex items-center justify-center rounded-full p-1.5 transition-colors ${
							isActive
								? "bg-surface-sunken text-foreground"
								: "text-muted-foreground hover:bg-surface-hover hover:text-foreground"
						}`}
					>
						<Icon className="h-4 w-4" />
					</button>
				);
			})}
		</div>
	);
}
