"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ApproximateRegion, EditScript, Operation, OpKind } from "@/lib/api/client";
import { type HighlightToken, tokenizeLines } from "@/lib/highlight";
import { type DiffSpan, inlineDiff } from "@/lib/inline-diff";

interface DiffViewProps {
	left: string;
	right: string;
	editScript: EditScript;
	language?: string;
	focusLines?: { left: number; right: number } | null;
}

interface LineAnnotation {
	kind: OpKind;
	label?: string;
}

const OP_COLORS: Record<OpKind, { bg: string; accent: string }> = {
	insert: { bg: "var(--diff-add-bg)", accent: "var(--diff-add)" },
	delete: { bg: "var(--diff-remove-bg)", accent: "var(--diff-remove)" },
	move: { bg: "var(--diff-move-bg)", accent: "var(--diff-move)" },
	rename: { bg: "var(--diff-change-bg)", accent: "var(--diff-change)" },
	update: { bg: "var(--diff-change-bg)", accent: "var(--diff-change)" },
	align: { bg: "var(--diff-move-bg)", accent: "var(--diff-move)" },
};

const CONTEXT_LINES = 3;
const LINE_HEIGHT = 20;
const ARROW_GAP = 56;
const COLLAPSED_HEIGHT = 28;
const OVERSCAN = 20;

function buildLineAnnotations(
	ops: Operation[],
	side: "left" | "right",
): Map<number, LineAnnotation> {
	const annotations = new Map<number, LineAnnotation>();

	for (const op of ops) {
		const node = side === "left" ? op.left_node : op.right_node;
		if (!node) continue;

		if (side === "left" && op.kind === "insert") continue;
		if (side === "right" && op.kind === "delete") continue;

		const line = node.location.line;
		const existing = annotations.get(line);
		if (!existing || opPriority(op.kind) > opPriority(existing.kind)) {
			annotations.set(line, { kind: op.kind, label: node.label });
		}
	}

	return annotations;
}

function opPriority(kind: OpKind): number {
	switch (kind) {
		case "move":
			return 5;
		case "delete":
			return 4;
		case "insert":
			return 4;
		case "rename":
			return 3;
		case "update":
			return 2;
		case "align":
			return 1;
	}
}

interface MoveArrow {
	leftLine: number;
	rightLine: number;
	label: string;
}

function buildMoveArrows(ops: Operation[]): MoveArrow[] {
	const seen = new Set<string>();
	const arrows: MoveArrow[] = [];
	for (const op of ops) {
		if (op.kind === "move" && op.left_node && op.right_node) {
			const key = `${op.left_node.location.line}-${op.right_node.location.line}`;
			if (seen.has(key)) continue;
			seen.add(key);
			arrows.push({
				leftLine: op.left_node.location.line,
				rightLine: op.right_node.location.line,
				label: op.left_node.label ?? op.left_node.kind,
			});
		}
	}
	return arrows;
}

function buildApproximateLines(
	regions: ApproximateRegion[] | undefined,
	side: "left" | "right",
): Set<number> {
	const lines = new Set<number>();
	if (!regions) return lines;
	for (const r of regions) {
		const span = side === "left" ? r.left_span : r.right_span;
		for (let ln = span.start.line; ln <= span.end.line; ln++) {
			lines.add(ln);
		}
	}
	return lines;
}

function buildLinePairs(ops: Operation[]): Map<number, number> {
	const pairs = new Map<number, number>();
	for (const op of ops) {
		if (op.left_node && op.right_node) {
			pairs.set(op.left_node.location.line, op.right_node.location.line);
		}
	}
	return pairs;
}

type Region =
	| { type: "lines"; startLine: number; endLine: number }
	| { type: "collapsed"; startLine: number; endLine: number; count: number };

function buildRegions(lineCount: number, annotations: Map<number, LineAnnotation>): Region[] {
	if (lineCount === 0) return [];

	const changed = new Set<number>();
	for (const line of annotations.keys()) {
		for (let c = line - CONTEXT_LINES; c <= line + CONTEXT_LINES; c++) {
			if (c >= 1 && c <= lineCount) changed.add(c);
		}
	}

	if (annotations.size === 0) {
		return [{ type: "collapsed", startLine: 1, endLine: lineCount, count: lineCount }];
	}

	const regions: Region[] = [];
	let i = 1;
	while (i <= lineCount) {
		if (changed.has(i)) {
			const start = i;
			while (i <= lineCount && changed.has(i)) i++;
			regions.push({ type: "lines", startLine: start, endLine: i - 1 });
		} else {
			const start = i;
			while (i <= lineCount && !changed.has(i)) i++;
			regions.push({
				type: "collapsed",
				startLine: start,
				endLine: i - 1,
				count: i - start,
			});
		}
	}
	return regions;
}

interface VirtualItem {
	key: string;
	offset: number;
	height: number;
	type: "line" | "collapsed";
	lineNum?: number;
	region?: Region;
}

function buildVirtualItems(regions: Region[], expanded: Set<number>): VirtualItem[] {
	const items: VirtualItem[] = [];
	let offset = 0;
	for (const region of regions) {
		if (region.type === "lines" || expanded.has(region.startLine)) {
			const end = region.type === "lines" ? region.endLine : region.endLine;
			for (let ln = region.startLine; ln <= end; ln++) {
				items.push({
					key: `l-${ln}`,
					offset,
					height: LINE_HEIGHT,
					type: "line",
					lineNum: ln,
					region,
				});
				offset += LINE_HEIGHT;
			}
		} else {
			items.push({
				key: `c-${region.startLine}`,
				offset,
				height: COLLAPSED_HEIGHT,
				type: "collapsed",
				region,
			});
			offset += COLLAPSED_HEIGHT;
		}
	}
	return items;
}

function getVisualY(lineNum: number, regions: Region[], expanded: Set<number>): number | null {
	let y = 0;
	for (const region of regions) {
		if (region.type === "lines" || expanded.has(region.startLine)) {
			if (lineNum >= region.startLine && lineNum <= region.endLine) {
				return y + (lineNum - region.startLine + 0.5) * LINE_HEIGHT;
			}
			y += (region.endLine - region.startLine + 1) * LINE_HEIGHT;
		} else {
			if (lineNum >= region.startLine && lineNum <= region.endLine) {
				return null;
			}
			y += COLLAPSED_HEIGHT;
		}
	}
	return null;
}

interface MergedSpan {
	content: string;
	dark?: string;
	light?: string;
	isDiffHighlight?: boolean;
}

function mergeSyntaxAndDiff(
	syntaxTokens: HighlightToken[],
	diffSpans: DiffSpan[],
	side: "left" | "right",
): MergedSpan[] {
	const filterType = side === "left" ? "delete" : "insert";
	const merged: MergedSpan[] = [];

	let sIdx = 0;
	let sOff = 0;
	let dIdx = 0;
	let dOff = 0;

	while (sIdx < syntaxTokens.length && dIdx < diffSpans.length) {
		const st = syntaxTokens[sIdx];
		const ds = diffSpans[dIdx];
		if (!st || !ds) break;
		const sRemain = st.content.length - sOff;
		const dRemain = ds.text.length - dOff;
		const take = Math.min(sRemain, dRemain);

		if (ds.type === filterType) {
			merged.push({
				content: st.content.slice(sOff, sOff + take),
				dark: st.dark,
				light: st.light,
				isDiffHighlight: true,
			});
		} else if (ds.type === "equal") {
			merged.push({
				content: st.content.slice(sOff, sOff + take),
				dark: st.dark,
				light: st.light,
			});
		}

		sOff += take;
		dOff += take;
		if (sOff >= st.content.length) {
			sIdx++;
			sOff = 0;
		}
		if (dOff >= ds.text.length) {
			dIdx++;
			dOff = 0;
		}
	}

	while (sIdx < syntaxTokens.length) {
		const st = syntaxTokens[sIdx];
		if (!st) break;
		merged.push({
			content: st.content.slice(sOff),
			dark: st.dark,
			light: st.light,
		});
		sIdx++;
		sOff = 0;
	}

	return merged;
}

function renderSyntaxTokens(tokens: HighlightToken[]): React.ReactNode {
	return tokens.map((t, i) => (
		<span key={i} className="sh" style={{ "--d": t.dark, "--l": t.light } as React.CSSProperties}>
			{t.content}
		</span>
	));
}

function renderMergedSpans(spans: MergedSpan[]): React.ReactNode {
	return spans.map((s, i) => (
		<span
			key={i}
			className={s.isDiffHighlight ? "sh diff-token-highlight" : "sh"}
			style={{ "--d": s.dark, "--l": s.light } as React.CSSProperties}
		>
			{s.content}
		</span>
	));
}

function CodeLine({
	lineNum,
	text,
	annotation,
	diffSpans,
	syntaxTokens,
	side,
	highlighted,
	approximate,
}: {
	lineNum: number;
	text: string;
	annotation: LineAnnotation | undefined;
	diffSpans?: DiffSpan[];
	syntaxTokens?: HighlightToken[];
	side?: "left" | "right";
	highlighted?: boolean;
	approximate?: boolean;
}) {
	const colors = annotation ? OP_COLORS[annotation.kind] : null;

	let content: React.ReactNode;
	if (diffSpans && side && syntaxTokens) {
		content = renderMergedSpans(mergeSyntaxAndDiff(syntaxTokens, diffSpans, side));
	} else if (syntaxTokens) {
		content = renderSyntaxTokens(syntaxTokens);
	} else if (diffSpans && side) {
		const filterType = side === "left" ? "delete" : "insert";
		content = diffSpans.map((span, i) => {
			if (span.type === "equal") {
				return <span key={i}>{span.text}</span>;
			}
			if (span.type === filterType) {
				return (
					<span key={i} className="diff-token-highlight">
						{span.text}
					</span>
				);
			}
			return null;
		});
	} else {
		content = text;
	}

	const className = `diff-line${highlighted ? " diff-line-highlighted" : ""}${approximate ? " diff-line-approximate" : ""}`;

	return (
		<div
			className={className}
			style={{
				background: approximate && !colors ? "var(--diff-approximate-bg)" : colors?.bg,
				borderLeft: colors
					? `2px solid ${colors.accent}`
					: approximate
						? "2px dashed var(--foreground-faint)"
						: "2px solid transparent",
				height: LINE_HEIGHT,
			}}
		>
			<span className="diff-gutter">{lineNum}</span>
			<span className="diff-code">{content}</span>
		</div>
	);
}

function CollapsedRegion({ count, onExpand }: { count: number; onExpand: () => void }) {
	return (
		<button
			type="button"
			className="diff-collapsed"
			onClick={onExpand}
			style={{ height: COLLAPSED_HEIGHT }}
		>
			{count} unchanged {count === 1 ? "line" : "lines"}
		</button>
	);
}

export function DiffView({ left, right, editScript, language, focusLines }: DiffViewProps) {
	const leftLines = useMemo(() => left.split("\n"), [left]);
	const rightLines = useMemo(() => right.split("\n"), [right]);
	const operations = useMemo(() => editScript.operations ?? [], [editScript.operations]);
	const leftAnnotations = useMemo(() => buildLineAnnotations(operations, "left"), [operations]);
	const rightAnnotations = useMemo(() => buildLineAnnotations(operations, "right"), [operations]);
	const moveArrows = useMemo(() => buildMoveArrows(operations), [operations]);
	const linePairs = useMemo(() => buildLinePairs(operations), [operations]);
	const leftApproximate = useMemo(
		() => buildApproximateLines(editScript.approximate, "left"),
		[editScript.approximate],
	);
	const rightApproximate = useMemo(
		() => buildApproximateLines(editScript.approximate, "right"),
		[editScript.approximate],
	);

	const [leftTokens, setLeftTokens] = useState<HighlightToken[][] | null>(null);
	const [rightTokens, setRightTokens] = useState<HighlightToken[][] | null>(null);

	useEffect(() => {
		if (!language) return;
		let cancelled = false;
		tokenizeLines(left, language).then((t) => {
			if (!cancelled) setLeftTokens(t);
		});
		tokenizeLines(right, language).then((t) => {
			if (!cancelled) setRightTokens(t);
		});
		return () => {
			cancelled = true;
		};
	}, [left, right, language]);

	const inlineSpans = useMemo(() => {
		const cache = new Map<string, DiffSpan[]>();
		for (const [leftLn, rightLn] of linePairs) {
			const leftText = leftLines[leftLn - 1] ?? "";
			const rightText = rightLines[rightLn - 1] ?? "";
			if (leftText !== rightText) {
				const spans = inlineDiff(leftText, rightText);
				const totalLen = spans.reduce((s, sp) => s + sp.text.length, 0);
				const equalLen = spans
					.filter((sp) => sp.type === "equal")
					.reduce((s, sp) => s + sp.text.length, 0);
				if (totalLen > 0 && equalLen / totalLen < 0.3) continue;
				const key = `${leftLn}:${rightLn}`;
				cache.set(key, spans);
			}
		}
		return cache;
	}, [linePairs, leftLines, rightLines]);

	const leftRegions = useMemo(
		() => buildRegions(leftLines.length, leftAnnotations),
		[leftLines.length, leftAnnotations],
	);
	const rightRegions = useMemo(
		() => buildRegions(rightLines.length, rightAnnotations),
		[rightLines.length, rightAnnotations],
	);

	const [expandedLeft, setExpandedLeft] = useState<Set<number>>(new Set());
	const [expandedRight, setExpandedRight] = useState<Set<number>>(new Set());
	const [hoveredArrowKey, setHoveredArrowKey] = useState<string | null>(null);
	const [scrollTop, setScrollTop] = useState(0);
	const [viewportHeight, setViewportHeight] = useState(600);

	const expandLeft = useCallback((startLine: number) => {
		setExpandedLeft((prev) => new Set(prev).add(startLine));
	}, []);
	const expandRight = useCallback((startLine: number) => {
		setExpandedRight((prev) => new Set(prev).add(startLine));
	}, []);

	const leftPaneRef = useRef<HTMLDivElement>(null);
	const rightPaneRef = useRef<HTMLDivElement>(null);
	const arrowsRef = useRef<SVGSVGElement>(null);
	const echo = useRef(false);

	useEffect(() => {
		if (!leftPaneRef.current) return;
		const observer = new ResizeObserver((entries) => {
			for (const entry of entries) {
				setViewportHeight(entry.contentRect.height);
			}
		});
		observer.observe(leftPaneRef.current);
		return () => observer.disconnect();
	}, []);

	const syncScroll = useCallback((src: HTMLDivElement | null, dst: HTMLDivElement | null) => {
		if (src) {
			setScrollTop(src.scrollTop);
			if (arrowsRef.current) {
				arrowsRef.current.style.transform = `translateY(${-src.scrollTop}px)`;
			}
		}
		if (echo.current) {
			echo.current = false;
			return;
		}
		if (src && dst && dst.scrollTop !== src.scrollTop) {
			echo.current = true;
			dst.scrollTop = src.scrollTop;
		}
	}, []);

	const handleLeftScroll = useCallback(
		() => syncScroll(leftPaneRef.current, rightPaneRef.current),
		[syncScroll],
	);
	const handleRightScroll = useCallback(
		() => syncScroll(rightPaneRef.current, leftPaneRef.current),
		[syncScroll],
	);

	useEffect(() => {
		if (!focusLines) return;
		const leftY = getVisualY(focusLines.left, leftRegions, expandedLeft);
		const rightY = getVisualY(focusLines.right, rightRegions, expandedRight);
		if (leftY !== null && leftPaneRef.current) {
			leftPaneRef.current.scrollTop = Math.max(0, leftY - 60);
		}
		if (rightY !== null && rightPaneRef.current) {
			rightPaneRef.current.scrollTop = Math.max(0, rightY - 60);
		}
	}, [focusLines, leftRegions, rightRegions, expandedLeft, expandedRight]);

	const leftItems = useMemo(
		() => buildVirtualItems(leftRegions, expandedLeft),
		[leftRegions, expandedLeft],
	);
	const rightItems = useMemo(
		() => buildVirtualItems(rightRegions, expandedRight),
		[rightRegions, expandedRight],
	);

	const lastLeft = leftItems[leftItems.length - 1];
	const leftTotalHeight = lastLeft ? lastLeft.offset + lastLeft.height : 0;
	const lastRight = rightItems[rightItems.length - 1];
	const rightTotalHeight = lastRight ? lastRight.offset + lastRight.height : 0;
	const contentHeight = Math.max(leftTotalHeight, rightTotalHeight);
	const paneMaxHeight = "calc(100vh - 200px)";

	const reversePairs = useMemo(() => {
		const rp = new Map<number, number>();
		for (const [leftLn, rightLn] of linePairs) rp.set(rightLn, leftLn);
		return rp;
	}, [linePairs]);

	const getSpans = (side: "left" | "right", ln: number): DiffSpan[] | undefined => {
		if (side === "left") {
			const rightLn = linePairs.get(ln);
			if (rightLn !== undefined) return inlineSpans.get(`${ln}:${rightLn}`);
		} else {
			const leftLn = reversePairs.get(ln);
			if (leftLn !== undefined) return inlineSpans.get(`${leftLn}:${ln}`);
		}
		return undefined;
	};

	const highlightedLines = useMemo(() => {
		if (!hoveredArrowKey) return { left: -1, right: -1 };
		const [l, r] = hoveredArrowKey.split("-").map(Number);
		return { left: l ?? -1, right: r ?? -1 };
	}, [hoveredArrowKey]);

	const visibleTop = scrollTop - OVERSCAN * LINE_HEIGHT;
	const visibleBottom = scrollTop + viewportHeight + OVERSCAN * LINE_HEIGHT;

	const renderVirtualPane = (
		items: VirtualItem[],
		lines: string[],
		annotations: Map<number, LineAnnotation>,
		onExpand: (startLine: number) => void,
		side: "left" | "right",
		tokens: HighlightToken[][] | null,
		totalHeight: number,
		approxLines: Set<number>,
	) => {
		const hlLine = side === "left" ? highlightedLines.left : highlightedLines.right;
		const visibleItems = items.filter(
			(item) => item.offset + item.height > visibleTop && item.offset < visibleBottom,
		);

		return (
			<div style={{ height: totalHeight, position: "relative" }}>
				{visibleItems.map((item) => (
					<div
						key={item.key}
						style={{
							position: "absolute",
							top: item.offset,
							left: 0,
							right: 0,
							height: item.height,
						}}
					>
						{item.type === "line" && item.lineNum !== undefined ? (
							<CodeLine
								lineNum={item.lineNum}
								text={lines[item.lineNum - 1] ?? ""}
								annotation={annotations.get(item.lineNum)}
								diffSpans={getSpans(side, item.lineNum)}
								syntaxTokens={tokens?.[item.lineNum - 1]}
								side={side}
								highlighted={item.lineNum === hlLine}
								approximate={approxLines.has(item.lineNum)}
							/>
						) : item.region ? (
							<CollapsedRegion
								count={item.region.endLine - item.region.startLine + 1}
								onExpand={() => onExpand(item.region?.startLine ?? 0)}
							/>
						) : null}
					</div>
				))}
			</div>
		);
	};

	return (
		<div className="diff-container">
			{/* Left pane */}
			<div ref={leftPaneRef} onScroll={handleLeftScroll} className="diff-pane">
				{renderVirtualPane(
					leftItems,
					leftLines,
					leftAnnotations,
					expandLeft,
					"left",
					leftTokens,
					contentHeight,
					leftApproximate,
				)}
			</div>

			{/* Move arrows */}
			<div className="diff-arrows" style={{ height: `min(${contentHeight}px, ${paneMaxHeight})` }}>
				<svg
					ref={arrowsRef}
					role="img"
					aria-label="Move arrows connecting relocated code between panes"
					width={ARROW_GAP}
					height={contentHeight}
					style={{ display: "block", position: "absolute", top: 0, left: 0 }}
				>
					{moveArrows.map((arrow) => {
						const y1 = getVisualY(arrow.leftLine, leftRegions, expandedLeft);
						const y2 = getVisualY(arrow.rightLine, rightRegions, expandedRight);
						if (y1 === null || y2 === null) return null;

						const key = `${arrow.leftLine}-${arrow.rightLine}`;
						const curve = `M 0 ${y1} C ${ARROW_GAP * 0.4} ${y1}, ${ARROW_GAP * 0.6} ${y2}, ${ARROW_GAP} ${y2}`;

						return (
							<g
								key={key}
								className="diff-arrow-group"
								onPointerEnter={() => setHoveredArrowKey(key)}
								onPointerLeave={() => setHoveredArrowKey(null)}
							>
								<title>{`moved: ${arrow.label}`}</title>
								{/* Wide invisible hit area */}
								<path
									d={curve}
									fill="none"
									stroke="transparent"
									strokeWidth={16}
									pointerEvents="stroke"
								/>
								<path
									d={curve}
									fill="none"
									stroke="var(--diff-move)"
									strokeWidth={2}
									className="diff-arrow-path"
								/>
								<circle cx={0} cy={y1} r={4} fill="var(--diff-move)" className="diff-arrow-dot" />
								<circle
									cx={ARROW_GAP}
									cy={y2}
									r={4}
									fill="var(--diff-move)"
									className="diff-arrow-dot"
								/>
							</g>
						);
					})}
				</svg>
			</div>

			{/* Right pane */}
			<div ref={rightPaneRef} onScroll={handleRightScroll} className="diff-pane">
				{renderVirtualPane(
					rightItems,
					rightLines,
					rightAnnotations,
					expandRight,
					"right",
					rightTokens,
					contentHeight,
					rightApproximate,
				)}
			</div>
		</div>
	);
}
