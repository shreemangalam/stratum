"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
	createMerge,
	type MergeDecision,
	type MergeEntry,
	type MergeResponse,
} from "@/lib/api/client";
import { type HighlightToken, tokenizeLines } from "@/lib/highlight";

const EXT_TO_LANG: Record<string, string> = {
	go: "go",
	js: "javascript",
	jsx: "javascript",
	ts: "typescript",
	tsx: "typescript",
	py: "python",
	java: "java",
	c: "c",
	h: "c",
	cpp: "cpp",
	cxx: "cpp",
	cc: "cpp",
	hpp: "cpp",
	xml: "xslt",
	xsl: "xslt",
	xslt: "xslt",
};

const LANGUAGE_OPTIONS = [
	{ value: "auto", label: "Auto-detect" },
	{ value: "go", label: "Go" },
	{ value: "javascript", label: "JavaScript" },
	{ value: "typescript", label: "TypeScript" },
	{ value: "python", label: "Python" },
	{ value: "java", label: "Java" },
	{ value: "c", label: "C" },
	{ value: "cpp", label: "C++" },
	{ value: "xslt", label: "XSLT / XML" },
];

function detectLanguageFromContent(code: string): string | null {
	const trimmed = code.trim();
	if (!trimmed) return null;
	const first500 = trimmed.slice(0, 500);
	if (/^package\s+\w+/.test(first500) || /\bfunc\s+\w+\s*\(/.test(first500)) return "go";
	if (/^<\?xml|<xsl:|<xsl:stylesheet/i.test(first500)) return "xslt";
	if (/^import\s+.*\bfrom\b/m.test(first500) && /:\s*(string|number|boolean|any)\b/.test(first500))
		return "typescript";
	if (/^#include\s*[<"]/.test(first500) || /\bstd::/.test(first500)) return "cpp";
	if (/\bpublic\s+class\b/.test(first500)) return "java";
	if (/^def\s+\w+|^class\s+\w+.*:$/m.test(first500) || /^import\s+\w+$/m.test(first500))
		return "python";
	if (/\b(?:const|let|var)\s+\w+\s*=/.test(first500) || /\bfunction\s+\w+\s*\(/.test(first500))
		return "javascript";
	return null;
}

function SyntaxBackdrop({ tokens }: { tokens: HighlightToken[][] | null }) {
	if (!tokens) return null;
	return (
		<>
			{tokens.map((line, i) => (
				<div key={i} className="code-editor-line">
					{line.map((t, j) => (
						<span
							key={j}
							className="sh"
							style={{ "--d": t.dark, "--l": t.light } as React.CSSProperties}
						>
							{t.content}
						</span>
					))}
					{"\n"}
				</div>
			))}
		</>
	);
}

function MergeCodeInput({
	id,
	label,
	value,
	onChange,
	onFileLoaded,
	placeholder,
	highlightLang,
}: {
	id: string;
	label: string;
	value: string;
	onChange: (v: string) => void;
	onFileLoaded?: (filename: string) => void;
	placeholder: string;
	highlightLang?: string;
}) {
	const [dragging, setDragging] = useState(false);
	const [filename, setFilename] = useState<string | null>(null);
	const fileRef = useRef<HTMLInputElement>(null);
	const textareaRef = useRef<HTMLTextAreaElement>(null);
	const backdropRef = useRef<HTMLPreElement>(null);
	const [tokens, setTokens] = useState<HighlightToken[][] | null>(null);

	useEffect(() => {
		if (!highlightLang || !value.trim()) {
			setTokens(null);
			return;
		}
		let cancelled = false;
		const timer = setTimeout(() => {
			tokenizeLines(value, highlightLang).then((t) => {
				if (!cancelled) setTokens(t);
			});
		}, 150);
		return () => {
			cancelled = true;
			clearTimeout(timer);
		};
	}, [value, highlightLang]);

	const syncScroll = useCallback(() => {
		if (textareaRef.current && backdropRef.current) {
			backdropRef.current.scrollTop = textareaRef.current.scrollTop;
			backdropRef.current.scrollLeft = textareaRef.current.scrollLeft;
		}
	}, []);

	const loadFile = useCallback(
		(file: File) => {
			setFilename(file.name);
			file.text().then((text) => {
				onChange(text);
				onFileLoaded?.(file.name);
			});
		},
		[onChange, onFileLoaded],
	);

	const wantsHighlight = !!highlightLang && value.trim().length > 0;
	const hasTokens = tokens !== null && value.trim().length > 0;

	return (
		// biome-ignore lint/a11y/noStaticElementInteractions: drop zone
		<div
			className={`code-drop-zone${dragging ? " code-drop-active" : ""}`}
			onDragOver={(e) => {
				e.preventDefault();
				setDragging(true);
			}}
			onDragLeave={() => setDragging(false)}
			onDrop={(e) => {
				e.preventDefault();
				setDragging(false);
				const file = e.dataTransfer.files[0];
				if (file) loadFile(file);
			}}
		>
			<div className="code-input-header">
				<label className="field-label" htmlFor={id}>
					{label}
				</label>
				<div className="code-input-actions">
					{filename && <span className="code-filename">{filename}</span>}
					<button
						type="button"
						className="btn btn-secondary btn-sm"
						onClick={() => fileRef.current?.click()}
					>
						Upload
					</button>
					<input
						ref={fileRef}
						type="file"
						hidden
						onChange={(e) => {
							const file = e.target.files?.[0];
							if (file) loadFile(file);
							e.target.value = "";
						}}
					/>
				</div>
			</div>
			<div className="code-editor-wrapper">
				<pre ref={backdropRef} className="code-editor-backdrop" aria-hidden="true">
					{hasTokens ? <SyntaxBackdrop tokens={tokens} /> : value || undefined}
				</pre>
				<textarea
					ref={textareaRef}
					id={id}
					className={`code-input${wantsHighlight ? " code-input-highlighted" : ""}`}
					value={value}
					onChange={(e) => {
						onChange(e.target.value);
						if (filename) setFilename(null);
					}}
					onScroll={syncScroll}
					placeholder={placeholder}
					spellCheck={false}
				/>
			</div>
			{dragging && <div className="code-drop-overlay">Drop file here</div>}
		</div>
	);
}

const DECISION_LABELS: Record<MergeDecision, string> = {
	unchanged: "Unchanged",
	"take-left": "Take left",
	"take-right": "Take right",
	"take-either": "Take either",
	delete: "Delete",
	conflict: "Conflict",
};

const DECISION_CLASSES: Record<MergeDecision, string> = {
	unchanged: "merge-unchanged",
	"take-left": "merge-take-left",
	"take-right": "merge-take-right",
	"take-either": "merge-take-either",
	delete: "merge-delete",
	conflict: "merge-conflict",
};

function MergeResultView({ result }: { result: MergeResponse }) {
	const { plan } = result;
	const total = plan.entries.length;
	const conflicts = plan.conflict_count;
	const auto = total - conflicts;

	return (
		<div className="merge-result">
			<div className="merge-summary">
				<span className="merge-summary-item">
					<strong>{total}</strong> {total === 1 ? "entry" : "entries"}
				</span>
				<span className="merge-summary-item merge-summary-auto">
					<strong>{auto}</strong> auto-resolved
				</span>
				{conflicts > 0 && (
					<span className="merge-summary-item merge-summary-conflicts">
						<strong>{conflicts}</strong> {conflicts === 1 ? "conflict" : "conflicts"}
					</span>
				)}
			</div>
			<table className="merge-table">
				<thead>
					<tr>
						<th>Node</th>
						<th>Kind</th>
						<th>Decision</th>
						<th>Reason</th>
					</tr>
				</thead>
				<tbody>
					{plan.entries.map((entry: MergeEntry, i: number) => {
						const ref = entry.base_node ?? entry.left_node ?? entry.right_node;
						const label = ref?.label ?? ref?.kind ?? "-";
						const kind = ref?.kind ?? "-";
						return (
							<tr key={i} className={DECISION_CLASSES[entry.decision]}>
								<td className="merge-cell-label">{label}</td>
								<td className="merge-cell-kind">{kind}</td>
								<td>
									<span className={`merge-badge ${DECISION_CLASSES[entry.decision]}`}>
										{DECISION_LABELS[entry.decision]}
									</span>
									{entry.conflict_kind && (
										<span className="merge-conflict-kind">{entry.conflict_kind}</span>
									)}
								</td>
								<td className="merge-cell-reason">{entry.reason}</td>
							</tr>
						);
					})}
				</tbody>
			</table>
		</div>
	);
}

export function MergeForm() {
	const [base, setBase] = useState("");
	const [left, setLeft] = useState("");
	const [right, setRight] = useState("");
	const [language, setLanguage] = useState("auto");
	const [error, setError] = useState<string | null>(null);
	const [submitting, setSubmitting] = useState(false);
	const [result, setResult] = useState<MergeResponse | null>(null);

	const resolvedLang =
		language === "auto"
			? (detectLanguageFromContent(base) ??
				detectLanguageFromContent(left) ??
				detectLanguageFromContent(right) ??
				null)
			: language;

	const detectLang = useCallback((filename: string) => {
		const ext = filename.split(".").pop()?.toLowerCase() ?? "";
		const detected = EXT_TO_LANG[ext];
		if (detected) setLanguage(detected);
	}, []);

	const handleSubmit = useCallback(async () => {
		if (!base.trim() || !left.trim() || !right.trim()) {
			setError("Base, left, and right code are all required");
			return;
		}

		let lang = language;
		if (lang === "auto") {
			lang =
				detectLanguageFromContent(base) ??
				detectLanguageFromContent(left) ??
				detectLanguageFromContent(right) ??
				"";
		}
		if (!lang) {
			setError("Could not detect language - please select one from the dropdown");
			return;
		}

		setSubmitting(true);
		setError(null);
		setResult(null);

		try {
			const resp = await createMerge({
				base: { content: base },
				left: { content: left },
				right: { content: right },
				language: lang,
			});
			setResult(resp);
		} catch (e) {
			setError(e instanceof Error ? e.message : "Unknown error");
		} finally {
			setSubmitting(false);
		}
	}, [base, left, right, language]);

	const handleKeyDown = useCallback(
		(e: React.KeyboardEvent) => {
			if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
				e.preventDefault();
				handleSubmit();
			}
		},
		[handleSubmit],
	);

	return (
		// biome-ignore lint/a11y/noStaticElementInteractions: keyboard shortcut wrapper
		<div onKeyDown={handleKeyDown}>
			<div className="merge-inputs">
				<MergeCodeInput
					id="base-input"
					label="Base"
					value={base}
					onChange={(v) => {
						setBase(v);
						setResult(null);
					}}
					onFileLoaded={detectLang}
					placeholder="Paste common ancestor code..."
					highlightLang={resolvedLang ?? undefined}
				/>
				<MergeCodeInput
					id="left-input"
					label="Left"
					value={left}
					onChange={(v) => {
						setLeft(v);
						setResult(null);
					}}
					onFileLoaded={detectLang}
					placeholder="Paste left side code..."
					highlightLang={resolvedLang ?? undefined}
				/>
				<MergeCodeInput
					id="right-input"
					label="Right"
					value={right}
					onChange={(v) => {
						setRight(v);
						setResult(null);
					}}
					onFileLoaded={detectLang}
					placeholder="Paste right side code..."
					highlightLang={resolvedLang ?? undefined}
				/>
			</div>

			<div className="form-footer">
				<div className="form-footer-left">
					<select
						className="merge-lang-select"
						value={language}
						onChange={(e) => {
							setLanguage(e.target.value);
							setError(null);
						}}
						aria-label="Language"
					>
						{LANGUAGE_OPTIONS.map((opt) => (
							<option key={opt.value} value={opt.value}>
								{opt.label}
							</option>
						))}
					</select>
					{language === "auto" && resolvedLang && (
						<span className="status-text">
							detected: {LANGUAGE_OPTIONS.find((o) => o.value === resolvedLang)?.label}
						</span>
					)}
				</div>

				{error && <span className="error-text">{error}</span>}

				<div className="form-footer-right">
					<kbd className="kbd-hint">Ctrl+Enter</kbd>
					<button
						type="button"
						className="btn btn-primary"
						onClick={handleSubmit}
						disabled={submitting || !base.trim() || !left.trim() || !right.trim()}
					>
						{submitting ? "Planning..." : "Merge"}
					</button>
				</div>
			</div>

			{result && <MergeResultView result={result} />}
		</div>
	);
}
