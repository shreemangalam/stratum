"use client";

import { useEffect, useState } from "react";
import { DiffForm } from "@/components/diff-form";
import { GitDiffForm } from "@/components/git-diff-form";
import { MergeForm } from "@/components/merge-form";

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const GIT_ENABLED =
	process.env.NEXT_PUBLIC_GIT_ENABLED === "true" ||
	(process.env.NODE_ENV === "development" && process.env.NEXT_PUBLIC_GIT_ENABLED !== "false");

export default function Home() {
	const [tab, setTab] = useState<"paste" | "git" | "merge">("paste");
	const [apiStatus, setApiStatus] = useState<"checking" | "ok" | "unreachable">("checking");

	useEffect(() => {
		let cancelled = false;
		async function check() {
			try {
				const res = await fetch(`${API_BASE}/api/v1/health`, { signal: AbortSignal.timeout(3000) });
				if (!cancelled) setApiStatus(res.ok ? "ok" : "unreachable");
			} catch {
				if (!cancelled) setApiStatus("unreachable");
			}
		}
		check();
		return () => {
			cancelled = true;
		};
	}, []);

	return (
		<div>
			{apiStatus === "unreachable" && (
				<div className="api-banner">
					<span>Cannot reach the API server at {API_BASE}</span>
					<button
						type="button"
						className="btn btn-secondary btn-sm"
						onClick={() => {
							setApiStatus("checking");
							fetch(`${API_BASE}/api/v1/health`, { signal: AbortSignal.timeout(3000) })
								.then((r) => setApiStatus(r.ok ? "ok" : "unreachable"))
								.catch(() => setApiStatus("unreachable"));
						}}
					>
						Retry
					</button>
				</div>
			)}
			<div className="tab-bar">
				<button
					type="button"
					className={`tab-btn${tab === "paste" ? " active" : ""}`}
					onClick={() => setTab("paste")}
				>
					Paste code
				</button>
				<button
					type="button"
					className={`tab-btn${tab === "merge" ? " active" : ""}`}
					onClick={() => setTab("merge")}
				>
					Merge
				</button>
				{GIT_ENABLED && (
					<button
						type="button"
						className={`tab-btn${tab === "git" ? " active" : ""}`}
						onClick={() => setTab("git")}
					>
						Git diff
					</button>
				)}
			</div>
			{tab === "paste" && <DiffForm />}
			{tab === "merge" && <MergeForm />}
			{tab === "git" && <GitDiffForm />}
		</div>
	);
}
