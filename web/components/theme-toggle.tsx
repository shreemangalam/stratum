"use client";

import { useEffect, useState } from "react";

type Theme = "light" | "dark";

function SunIcon() {
	return (
		<svg
			width="16"
			height="16"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			strokeWidth="1.5"
			strokeLinecap="round"
			aria-hidden="true"
		>
			<circle cx="12" cy="12" r="4" />
			<path d="M12 2v3M12 19v3M4.9 4.9l2.1 2.1M17 17l2.1 2.1M2 12h3M19 12h3M4.9 19.1L7 17M17 7l2.1-2.1" />
		</svg>
	);
}

function MoonIcon() {
	return (
		<svg
			width="16"
			height="16"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			strokeWidth="1.5"
			strokeLinecap="round"
			strokeLinejoin="round"
			aria-hidden="true"
		>
			<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" />
		</svg>
	);
}

// Client component: reads and writes the document theme attribute,
// which only exists in the browser.
export function ThemeToggle() {
	const [theme, setTheme] = useState<Theme | null>(null);

	useEffect(() => {
		const current = document.documentElement.getAttribute("data-theme");
		setTheme(current === "light" ? "light" : "dark");
	}, []);

	const toggle = () => {
		// Not yet mounted; the real theme is unknown, so do nothing.
		if (theme === null) return;
		const next: Theme = theme === "light" ? "dark" : "light";
		document.documentElement.setAttribute("data-theme", next);
		try {
			localStorage.setItem("stratum-theme", next);
		} catch {
			// Storage unavailable; the theme still applies for this page view.
		}
		setTheme(next);
	};

	// No `disabled` on the button: it differed between server and client
	// render (null vs true) and caused a hydration mismatch. The toggle
	// no-ops until mounted instead.
	return (
		<button
			type="button"
			className="btn btn-secondary btn-icon"
			onClick={toggle}
			aria-label={theme === "light" ? "Switch to dark theme" : "Switch to light theme"}
		>
			{theme === "light" ? <MoonIcon /> : <SunIcon />}
		</button>
	);
}
