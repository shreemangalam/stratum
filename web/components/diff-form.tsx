"use client";

import { useRouter } from "next/navigation";
import { type DragEvent, useCallback, useEffect, useRef, useState } from "react";
import { createDiff, type DiffJob } from "@/lib/api/client";
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

interface SelectOption {
	value: string;
	label: string;
}

function CustomSelect({
	options,
	value,
	onChange,
	ariaLabel,
}: {
	options: SelectOption[];
	value: string;
	onChange: (v: string) => void;
	ariaLabel: string;
}) {
	const [open, setOpen] = useState(false);
	const ref = useRef<HTMLDivElement>(null);
	const selected = options.find((o) => o.value === value);

	useEffect(() => {
		if (!open) return;
		const handler = (e: MouseEvent) => {
			if (ref.current && !ref.current.contains(e.target as Node)) {
				setOpen(false);
			}
		};
		document.addEventListener("mousedown", handler);
		return () => document.removeEventListener("mousedown", handler);
	}, [open]);

	const handleKey = useCallback(
		(e: React.KeyboardEvent) => {
			if (e.key === "Escape") {
				setOpen(false);
				return;
			}
			if (e.key === "Enter" || e.key === " ") {
				e.preventDefault();
				setOpen((o) => !o);
				return;
			}
			const idx = options.findIndex((o) => o.value === value);
			if (e.key === "ArrowDown") {
				e.preventDefault();
				if (!open) {
					setOpen(true);
					return;
				}
				const next = Math.min(idx + 1, options.length - 1);
				const nextOpt = options[next];
				if (nextOpt) onChange(nextOpt.value);
			} else if (e.key === "ArrowUp") {
				e.preventDefault();
				if (!open) {
					setOpen(true);
					return;
				}
				const prev = Math.max(idx - 1, 0);
				const prevOpt = options[prev];
				if (prevOpt) onChange(prevOpt.value);
			}
		},
		[open, options, value, onChange],
	);

	return (
		<div className="custom-select" ref={ref}>
			<button
				type="button"
				className="custom-select-trigger"
				onClick={() => setOpen((o) => !o)}
				onKeyDown={handleKey}
				aria-label={ariaLabel}
				aria-expanded={open}
				aria-haspopup="listbox"
			>
				{selected?.label ?? value}
				<span className="custom-select-arrow" aria-hidden>
					&#9662;
				</span>
			</button>
			{open && (
				<div className="custom-select-menu" role="listbox">
					{options.map((opt) => (
						<div
							key={opt.value}
							role="option"
							tabIndex={-1}
							aria-selected={opt.value === value}
							className={`custom-select-option${opt.value === value ? " selected" : ""}`}
							onMouseDown={(e) => {
								e.preventDefault();
								onChange(opt.value);
								setOpen(false);
							}}
						>
							{opt.label}
						</div>
					))}
				</div>
			)}
		</div>
	);
}

const LANGUAGE_OPTIONS: SelectOption[] = [
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
	if (/<\w+[^>]*xmlns/i.test(first500)) return "xslt";
	if (/^import\s+.*\bfrom\b/m.test(first500) && /:\s*(string|number|boolean|any)\b/.test(first500))
		return "typescript";
	if (/\binterface\s+\w+\s*\{/.test(first500) && /:\s*(string|number|boolean)\b/.test(first500))
		return "typescript";
	if (/^#include\s*[<"]/.test(first500) || /\bstd::/.test(first500)) return "cpp";
	if (/\bpublic\s+class\b/.test(first500) || /\bpublic\s+static\s+void\s+main\b/.test(first500))
		return "java";
	if (
		/^def\s+\w+|^class\s+\w+.*:$/m.test(first500) ||
		/^import\s+\w+$/m.test(first500) ||
		/^\s*print\s*\(/.test(first500)
	)
		return "python";
	if (
		/\b(?:const|let|var)\s+\w+\s*=/.test(first500) ||
		/\bfunction\s+\w+\s*\(/.test(first500) ||
		/=>\s*\{/.test(first500)
	)
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

function CodeInput({
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

	const handleDrop = useCallback(
		(e: DragEvent<HTMLDivElement>) => {
			e.preventDefault();
			setDragging(false);
			const file = e.dataTransfer.files[0];
			if (file) loadFile(file);
		},
		[loadFile],
	);

	const wantsHighlight = !!highlightLang && value.trim().length > 0;
	const hasTokens = tokens !== null && value.trim().length > 0;

	return (
		// biome-ignore lint/a11y/noStaticElementInteractions: drop zone requires drag event handlers
		<div
			className={`code-drop-zone${dragging ? " code-drop-active" : ""}`}
			onDragOver={(e) => {
				e.preventDefault();
				setDragging(true);
			}}
			onDragLeave={() => setDragging(false)}
			onDrop={handleDrop}
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
						Upload file
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

export function DiffForm() {
	const router = useRouter();
	const [left, setLeft] = useState("");
	const [right, setRight] = useState("");
	const [language, setLanguage] = useState("auto");
	const [error, setError] = useState<string | null>(null);
	const [submitting, setSubmitting] = useState(false);

	const resolvedLang =
		language === "auto"
			? (detectLanguageFromContent(left) ?? detectLanguageFromContent(right) ?? null)
			: language;

	const leftLines = left.split("\n").length;
	const rightLines = right.split("\n").length;
	const maxLines = Math.max(leftLines, rightLines);
	const isLargeFile = maxLines > 5000;

	const detectLang = useCallback((filename: string) => {
		const ext = filename.split(".").pop()?.toLowerCase() ?? "";
		const detected = EXT_TO_LANG[ext];
		if (detected) setLanguage(detected);
	}, []);

	const handleSubmit = useCallback(async () => {
		if (!left.trim() || !right.trim()) {
			setError("Both original and modified code are required");
			return;
		}

		let lang = language;
		if (lang === "auto") {
			lang = detectLanguageFromContent(left) ?? detectLanguageFromContent(right) ?? "";
		}

		if (!lang) {
			setError("Could not detect language - please select one from the dropdown");
			return;
		}

		setSubmitting(true);
		setError(null);

		try {
			const job: DiffJob = await createDiff({
				left: { content: left },
				right: { content: right },
				language: lang,
			});

			router.push(`/diffs/${job.id}`);
		} catch (e) {
			setError(e instanceof Error ? e.message : "Unknown error");
			setSubmitting(false);
		}
	}, [left, right, language, router]);

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
			<div
				style={{
					display: "grid",
					gridTemplateColumns: "1fr 1fr",
					gap: 16,
					marginBottom: 16,
				}}
			>
				<CodeInput
					id="left-input"
					label="Original"
					value={left}
					onChange={setLeft}
					onFileLoaded={detectLang}
					placeholder="Paste original code or drop a file..."
					highlightLang={resolvedLang ?? undefined}
				/>
				<CodeInput
					id="right-input"
					label="Modified"
					value={right}
					onChange={setRight}
					onFileLoaded={detectLang}
					placeholder="Paste modified code or drop a file..."
					highlightLang={resolvedLang ?? undefined}
				/>
			</div>

			<div className="form-footer">
				<div className="form-footer-left">
					<CustomSelect
						options={LANGUAGE_OPTIONS}
						value={language}
						onChange={(v) => {
							setLanguage(v);
							setError(null);
						}}
						ariaLabel="Language"
					/>
					{language === "auto" &&
						(left.trim() || right.trim()) &&
						(() => {
							const detected = detectLanguageFromContent(left) ?? detectLanguageFromContent(right);
							const label = detected
								? LANGUAGE_OPTIONS.find((o) => o.value === detected)?.label
								: null;
							return label ? (
								<span className="status-text">detected: {label}</span>
							) : left.trim() || right.trim() ? (
								<span className="warning-text">select a language</span>
							) : null;
						})()}
				</div>

				{error && <span className="error-text">{error}</span>}
				{isLargeFile && !error && (
					<span className="warning-text">
						Large file ({maxLines.toLocaleString()} lines) - processing may take longer
					</span>
				)}

				<div className="form-footer-right">
					<kbd className="kbd-hint">Ctrl+Enter</kbd>
					<button
						type="button"
						className="btn btn-primary"
						onClick={handleSubmit}
						disabled={submitting || !left.trim() || !right.trim()}
					>
						{submitting ? "Submitting..." : "Diff"}
					</button>
				</div>
			</div>
		</div>
	);
}
