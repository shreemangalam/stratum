"use client";

import { useParams, useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { DiffResult } from "@/components/diff-result";
import { type DiffJob, type EditScript, getDiff, streamDiff } from "@/lib/api/client";

export default function DiffPage() {
	const params = useParams<{ id: string }>();
	const router = useRouter();
	const id = params.id;

	const [job, setJob] = useState<DiffJob | null>(null);
	const [result, setResult] = useState<EditScript | null>(null);
	const [progress, setProgress] = useState<string | null>(null);
	const [error, setError] = useState<string | null>(null);
	const [retryCount, setRetryCount] = useState(0);

	const load = useCallback(async (diffId: string, signal: AbortSignal) => {
		setError(null);
		setProgress("Loading...");

		try {
			const fetched = await getDiff(diffId);
			if (signal.aborted) return;
			setJob(fetched);

			if (fetched.status === "completed" && fetched.result) {
				setResult(fetched.result);
				setProgress(null);
				return;
			}
			if (fetched.status === "failed") {
				setError(fetched.error ?? "Diff failed");
				setProgress(null);
				return;
			}

			setProgress(fetched.status);
			const cancel = streamDiff(diffId, (type, data) => {
				if (signal.aborted) return;
				if (type === "status") {
					setProgress(data);
				} else if (type === "progress") {
					setProgress(data);
				} else if (type === "result") {
					const parsed = JSON.parse(data) as EditScript;
					setResult(parsed);
					setProgress(null);
				} else if (type === "error") {
					setError(data);
					setProgress(null);
				}
			});

			signal.addEventListener("abort", cancel);
		} catch (e) {
			if (!signal.aborted) {
				setError(e instanceof Error ? e.message : "Failed to load diff");
				setProgress(null);
			}
		}
	}, []);

	// biome-ignore lint/correctness/useExhaustiveDependencies: retryCount is an intentional re-trigger
	useEffect(() => {
		if (!id) return;
		const controller = new AbortController();
		load(id, controller.signal);
		return () => controller.abort();
	}, [id, retryCount, load]);

	const handleRetry = useCallback(() => {
		setRetryCount((c) => c + 1);
	}, []);

	if (error) {
		return (
			<div className="diff-page-status">
				<div className="error-state">
					<p className="error-state-message">{error}</p>
					<div className="error-state-actions">
						<button type="button" className="btn btn-primary btn-sm" onClick={handleRetry}>
							Retry
						</button>
						<button
							type="button"
							className="btn btn-secondary btn-sm"
							onClick={() => router.push("/")}
						>
							New diff
						</button>
					</div>
				</div>
			</div>
		);
	}

	if (!result || !job) {
		return (
			<div className="diff-page-status">
				<div className="loading-indicator">
					<span className="loading-dot" />
					<span>{progress ?? "Loading..."}</span>
				</div>
			</div>
		);
	}

	const left = job.left_source ?? "";
	const right = job.right_source ?? "";

	if (!left && !right) {
		return (
			<div className="diff-page-status">
				<div className="error-state">
					<p className="error-state-message">Source code is no longer available for this diff.</p>
					<p className="error-state-hint">
						Sources are cleaned up after job completion to save storage.
					</p>
					<div className="error-state-actions">
						<button
							type="button"
							className="btn btn-secondary btn-sm"
							onClick={() => router.push("/")}
						>
							New diff
						</button>
					</div>
				</div>
			</div>
		);
	}

	return (
		<DiffResult
			left={left}
			right={right}
			editScript={result}
			language={job.language}
			onNewDiff={() => router.push("/")}
		/>
	);
}
