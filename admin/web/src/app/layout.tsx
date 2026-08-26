import type { Metadata } from "next";
import { GeistSans } from "geist/font/sans";
import { GeistMono } from "geist/font/mono";
import "./globals.css";

export const metadata: Metadata = {
	title: "Home Lab Admin",
	description: "Administration for the home lab databases and cluster",
};

// Runs before paint to apply the saved theme (light/dark/system) so the page
// never flashes the wrong colors on load. Kept inline and dependency-free, and
// mirrored by theme-switcher.tsx — change one and change the other.
const themeScript = `(function(){try{var t=localStorage.getItem('theme')||'system';var d=t==='dark'||(t==='system'&&window.matchMedia('(prefers-color-scheme: dark)').matches);var c=document.documentElement.classList;c.toggle('dark',d);c.toggle('light',!d);}catch(e){}})();`;

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
	return (
		// The theme script rewrites the class list before React hydrates, which is
		// the whole point of it — so the mismatch it causes is expected.
		<html lang="en" suppressHydrationWarning>
			<head>
				<script dangerouslySetInnerHTML={{ __html: themeScript }} />
			</head>
			<body className={`${GeistSans.variable} ${GeistMono.variable} antialiased`}>{children}</body>
		</html>
	);
}
