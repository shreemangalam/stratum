"use client";

import { usePathname } from "next/navigation";
import { useCallback, useMemo, useState } from "react";
import { DiffView } from "@/components/diff-view";
import type { EditScript, OpKind } from "@/lib/api/client";

const KIND_COLORS: Record<string, string> = {
	insert: "var(--diff-add)",
	delete: "var(--diff-remove)",
	move: "var(--diff-move)",
	rename: "var(--diff-change)",
	update: "var(--diff-change)",
	align: "var(--diff-move)",
};

const KIND_ORDER: OpKind[] = ["insert", "delete", "move", "rename", "update", "align"];

function countByKind(operations: EditScript["operations"]): Record<string, number> {
	const counts: Record<string, number> = {};
	for (const op of operations ?? []) {
		counts[op.kind] = (counts[op.kind] ?? 0) + 1;
	}
	return counts;
}

function nodeName(node: { label?: string; kind?: string } | undefined): string | null {
	if (!node) return null;
	const label = node.label || "";
	const kind = node.kind || "";
	if (kind !== "line" && label) return label;
	if (kind !== "line") return kind;
	// Line-based fallback: use truncated line content as label
	const trimmed = label.trim();
	if (!trimmed) return null;
	return trimmed.length > 40 ? `${trimmed.slice(0, 37)}...` : trimmed;
}

function summarizeChanges(editScript: EditScript): string[] {
	const ops = editScript.operations ?? [];
	const semantic = editScript.semantic ?? [];
	const lines: string[] = [];

	if (semantic.length > 0) {
		for (const sc of semantic) {
			const name = sc.right_node.label || sc.right_node.kind;
			if (sc.verdict === "behavior-preserving") {
				lines.push(`${name} - likely behavior-preserving: ${sc.reason}`);
			} else if (sc.verdict === "behavior-changing") {
				const leftName = sc.left_node.label || sc.left_node.kind;
				if (leftName !== name) {
					lines.push(`${leftName} renamed to ${name} - ${sc.reason}`);
				} else {
					lines.push(`${name} - ${sc.reason}`);
				}
			} else {
				lines.push(`${name} - ${sc.reason}`);
			}
		}
		return lines;
	}

	const renames = ops.filter((o) => o.kind === "rename");
	const moves = ops.filter((o) => o.kind === "move");
	const inserts = ops.filter((o) => o.kind === "insert");
	const deletes = ops.filter((o) => o.kind === "delete");
	const updates = ops.filter((o) => o.kind === "update");

	for (const r of renames) {
		const left = nodeName(r.left_node);
		const right = nodeName(r.right_node);
		if (left && right) {
			lines.push(`${left} renamed to ${right}`);
		}
	}
	if (moves.length <= 5) {
		for (const m of moves) {
			const name = nodeName(m.left_node);
			const from = m.left_node?.location?.line;
			const to = m.right_node?.location?.line;
			if (name && from && to) {
				lines.push(`${name} moved (line ${from} → ${to})`);
			} else if (name) {
				lines.push(`${name} moved`);
			}
		}
	} else {
		lines.push(`${moves.length} lines moved`);
	}
	if (inserts.length <= 3) {
		for (const ins of inserts) {
			const name = nodeName(ins.right_node);
			if (name) lines.push(`${name} added`);
		}
	} else {
		lines.push(`${inserts.length} lines added`);
	}
	if (deletes.length <= 3) {
		for (const del of deletes) {
			const name = nodeName(del.left_node);
			if (name) lines.push(`${name} removed`);
		}
	} else {
		lines.push(`${deletes.length} lines removed`);
	}
	if (updates.length > 0)
		lines.push(`${updates.length} value${updates.length > 1 ? "s" : ""} changed`);

	return lines;
}

interface DiffResultProps {
	left: string;
	right: string;
	editScript: EditScript;
	language?: string;
	onNewDiff?: () => void;
}

export function DiffResult({ left, right, editScript, language, onNewDiff }: DiffResultProps) {
	const [focusLines, setFocusLines] = useState<{ left: number; right: number } | null>(null);
	const [copied, setCopied] = useState<"link" | "summary" | null>(null);
	const pathname = usePathname();

	const opCounts = useMemo(() => countByKind(editScript.operations), [editScript.operations]);

	const summary = useMemo(() => summarizeChanges(editScript), [editScript]);

	const total = (editScript.operations ?? []).length;

	const isPermalink = pathname.startsWith("/diffs/");

	const copyLink = useCallback(async () => {
		try {
			await navigator.clipboard.writeText(window.location.href);
			setCopied("link");
			setTimeout(() => setCopied(null), 2000);
		} catch {
			/* clipboard unavailable */
		}
	}, []);

	const copySummary = useCallback(async () => {
		const counts = KIND_ORDER.filter((k) => opCounts[k])
			.map((k) => `${opCounts[k]} ${k}`)
			.join(", ");
		const header = `Stratum diff: ${total} operations (${counts})${language ? ` [${language}]` : ""}`;
		const body = summary.length > 0 ? `\n${summary.map((l) => `  - ${l}`).join("\n")}` : "";
		const link = isPermalink ? `\n${window.location.href}` : "";
		try {
			await navigator.clipboard.writeText(header + body + link);
			setCopied("summary");
			setTimeout(() => setCopied(null), 2000);
		} catch {
			/* clipboard unavailable */
		}
	}, [total, opCounts, language, summary, isPermalink]);

	return (
		<div>
			<div className="result-header">
				<div className="result-stats">
					<span className="stat">
						<span className="stat-count">{total}</span> op{total !== 1 ? "s" : ""}
					</span>
					{KIND_ORDER.filter((k) => opCounts[k]).map((kind) => (
						<span key={kind} className="stat">
							<span className="stat-dot" style={{ background: KIND_COLORS[kind] }} />
							<span className="stat-count">{opCounts[kind]}</span>
							{kind}
						</span>
					))}
					{language && <span className="stat stat-lang">{language}</span>}
					{editScript.approximate && editScript.approximate.length > 0 && (
						<span className="stat stat-approximate">
							{editScript.approximate.length} approximate
						</span>
					)}
				</div>
				<div className="result-actions">
					{isPermalink && (
						<button type="button" className="btn btn-secondary btn-sm" onClick={copyLink}>
							{copied === "link" ? "Copied!" : "Copy link"}
						</button>
					)}
					<button type="button" className="btn btn-secondary btn-sm" onClick={copySummary}>
						{copied === "summary" ? "Copied!" : "Copy summary"}
					</button>
					{onNewDiff && (
						<button type="button" className="btn btn-secondary" onClick={onNewDiff}>
							New diff
						</button>
					)}
				</div>
			</div>

			{summary.length > 0 && (
				<div className="change-summary">
					<span className="change-summary-label">Summary</span>
					<ul className="change-summary-list">
						{summary.map((line, i) => (
							<li key={i}>{line}</li>
						))}
					</ul>
				</div>
			)}

			{editScript.semantic && editScript.semantic.length > 0 && (
				<div className="verdict-row">
					{editScript.semantic.map((sc) => (
						<button
							type="button"
							key={`${sc.left_node.id}-${sc.right_node.id}`}
							className="verdict-pill verdict-pill-clickable"
							onClick={() =>
								setFocusLines({
									left: sc.left_node.location.line,
									right: sc.right_node.location.line,
								})
							}
						>
							<span className="verdict-pill-label">
								{sc.right_node.label || sc.right_node.kind}
							</span>
							{sc.verdict} - {sc.reason}
						</button>
					))}
				</div>
			)}
			<DiffView
				left={left}
				right={right}
				editScript={editScript}
				language={language}
				focusLines={focusLines}
			/>
		</div>
	);
}
