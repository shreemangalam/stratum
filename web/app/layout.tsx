import "@fontsource-variable/inter";
import "@fontsource-variable/jetbrains-mono";
import type { Metadata } from "next";
import Link from "next/link";
import { ThemeToggle } from "@/components/theme-toggle";
import "./globals.css";

export const metadata: Metadata = {
	title: "Stratum",
	description: "Structural diff and merge tool",
};

// Runs before paint so the stored theme applies without a flash.
const themeInit = `(function(){var t;try{t=localStorage.getItem("stratum-theme")}catch(e){}if(t!=="light"&&t!=="dark"){t="dark"}document.documentElement.setAttribute("data-theme",t)})();`;

export default function RootLayout({ children }: LayoutProps<"/">) {
	return (
		<html lang="en" suppressHydrationWarning>
			<head>
				{/* biome-ignore lint/security/noDangerouslySetInnerHtml: static theme-init script, no user input */}
				<script dangerouslySetInnerHTML={{ __html: themeInit }} />
			</head>
			<body>
				<header className="app-header">
					<div>
						<span className="app-name">Stratum</span>
						<span className="app-tagline">Structural diff</span>
					</div>
					<div style={{ display: "flex", alignItems: "center", gap: 12 }}>
						<Link
							href="/help"
							className="app-tagline"
							style={{ borderBottom: "1px solid var(--border)" }}
						>
							Help
						</Link>
						<ThemeToggle />
					</div>
				</header>
				<main className="app-main">{children}</main>
			</body>
		</html>
	);
}
