import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
	title: "Stratum - Help",
	description: "How to use Stratum's structural diff",
};

const operations = [
	{
		name: "Insert",
		color: "var(--diff-add)",
		bg: "var(--diff-add-bg)",
		description:
			"A node exists in the modified version but not in the original. New functions, parameters, statements, or XML elements. Shown with a green left-edge bar.",
	},
	{
		name: "Delete",
		color: "var(--diff-remove)",
		bg: "var(--diff-remove-bg)",
		description:
			"A node exists in the original but not in the modified version. Removed functions, fields, or entire blocks. Shown with a red left-edge bar.",
	},
	{
		name: "Move",
		color: "var(--diff-move)",
		bg: "var(--diff-move-bg)",
		description:
			"A node exists in both versions but its position in the tree changed. Reordered functions, relocated class methods, swapped parameters. An orange curve connects the original and new positions across panes.",
	},
	{
		name: "Rename",
		color: "var(--diff-change)",
		bg: "var(--diff-change-bg)",
		description:
			"A node's identity changed but its structure is the same. A function renamed from 'processData' to 'transformData', a variable renamed, an XML element given a new tag name.",
	},
	{
		name: "Update",
		color: "var(--diff-change)",
		bg: "var(--diff-change-bg)",
		description:
			"A node's content changed while its identity held. Modified function bodies, changed attribute values, rewritten expressions. Inline word-level highlighting marks the specific tokens that differ.",
	},
];

const languages = [
	{
		name: "Go",
		parser: "go/parser (stdlib)",
		level: "Full AST",
		detail: "Functions, types, interfaces, methods, statements, expressions",
	},
	{
		name: "XSLT / XML",
		parser: "Custom XML parser",
		level: "Full element tree",
		detail: "Elements, attributes, templates, text nodes",
	},
	{
		name: "JavaScript",
		parser: "Structural scanner",
		level: "Structural",
		detail: "Functions, classes, methods, imports, exports, variables",
	},
	{
		name: "TypeScript",
		parser: "Structural scanner",
		level: "Structural",
		detail: "Functions, classes, interfaces, types, methods, imports",
	},
	{
		name: "Python",
		parser: "Structural scanner",
		level: "Structural",
		detail: "Functions, classes, methods, imports",
	},
	{
		name: "Java",
		parser: "Structural scanner",
		level: "Structural",
		detail: "Classes, interfaces, enums, methods, fields, imports",
	},
	{
		name: "C / C++",
		parser: "Structural scanner",
		level: "Structural",
		detail: "Functions, structs, unions, enums, typedefs, preprocessor directives",
	},
];

const algorithmSteps = [
	{
		name: "Parse",
		description:
			"Source code is parsed into a labeled, ordered tree. Each node carries a kind (function, class, statement), an optional label (name), its source span, and a Merkle hash of its subtree.",
	},
	{
		name: "Top-down match",
		description:
			"Starting at the roots, the algorithm walks both trees top-down, matching subtrees whose Merkle hashes are identical. These matches are anchors - subtrees that haven't changed at all.",
	},
	{
		name: "Bottom-up match",
		description:
			"Unmatched nodes are compared bottom-up using Dice's coefficient on their children's match sets. A node in the old tree is paired with the most-similar unmatched node of the same kind in the new tree, above a configurable threshold.",
	},
	{
		name: "Optimal alignment",
		description:
			"For small unmatched subtrees, an optimal alignment pass runs a bounded edit-distance to find the best correspondence. If the subtree exceeds the node budget, the algorithm degrades gracefully to line-level diff and marks the region approximate.",
	},
	{
		name: "Edit script",
		description:
			"The matched node pairs are compared to produce a sequence of operations: insert, delete, move, rename, update. Move detection is the key difference from line diff - a function that shifted position is one move, not a delete-plus-insert.",
	},
	{
		name: "Semantic classification",
		description:
			"Each matched function is classified as behavior-preserving, behavior-changing, or indeterminate using conservative edit-shape heuristics plus bounded Go dataflow analysis for statement reorderings.",
	},
];

export default function HelpPage() {
	return (
		<div className="help-page">
			<div className="help-nav">
				<Link href="/" className="help-back">
					Back to diff
				</Link>
			</div>

			<h1 className="help-title">How Stratum works</h1>
			<p className="help-intro">
				Stratum is a structural diff tool. Instead of comparing files line by line, it parses source
				code into a syntax tree and matches nodes across versions. The result is an edit script that
				names moves, renames, and semantic changes - not just &ldquo;line 42 changed.&rdquo;
			</p>
			<p className="help-text">
				A function that was relocated from line 10 to line 80 shows up as a single
				<strong> move</strong> operation, not as 8 deleted lines and 8 inserted lines. A renamed
				variable is one <strong>rename</strong>, not a dozen scattered changes. This makes code
				review faster and refactorings easier to verify.
			</p>

			<section className="help-section">
				<h2 className="help-heading">Quick start</h2>
				<p className="help-text" style={{ marginBottom: 8 }}>
					Stratum offers two ways to create a diff:
				</p>
				<p className="help-text">
					<strong>Paste code</strong> (default tab)
				</p>
				<ol className="help-steps">
					<li>
						Paste or drag-drop the original code into the left pane, modified code into the right.
					</li>
					<li>
						Select a language from the dropdown, or leave on <strong>Auto-detect</strong> to let
						Stratum identify it from the code.
					</li>
					<li>
						Click <strong>Diff</strong> or press <strong>Ctrl+Enter</strong>. The job runs
						asynchronously.
					</li>
					<li>The result opens on a shareable permalink page with the full diff view.</li>
				</ol>
				<p className="help-text" style={{ marginTop: 12 }}>
					<strong>Git diff</strong> (second tab)
				</p>
				<ol className="help-steps">
					<li>Enter the absolute path to a local git repository.</li>
					<li>
						Set the base ref (e.g. <code>HEAD~1</code>, <code>main</code>, a commit hash) and target
						ref.
					</li>
					<li>
						Click <strong>List files</strong> to see which files changed between those refs.
					</li>
					<li>
						Select a file and click <strong>Diff selected file</strong>.
					</li>
				</ol>
			</section>

			<section className="help-section">
				<h2 className="help-heading">Language auto-detection</h2>
				<p className="help-text">
					When set to <strong>Auto-detect</strong>, Stratum examines the first 500 characters of
					your code for language-specific patterns: <code>package</code>/<code>func</code> for Go,
					XML declarations for XSLT, type annotations for TypeScript, <code>#include</code> for C++,
					class declarations for Java, <code>def</code>/<code>import</code> for Python, and common
					JS patterns for JavaScript.
				</p>
				<p className="help-text">
					If auto-detection fails, Stratum asks you to select a language explicitly rather than
					guessing wrong. The code editor highlights your pasted code in the detected language in
					real time, so you can verify the detection before submitting.
				</p>
			</section>

			<section className="help-section">
				<h2 className="help-heading">Reading the results</h2>
				<p className="help-text">
					The <strong>result header</strong> shows the total number of operations and a breakdown by
					type, each with its colored indicator dot.
				</p>
				<p className="help-text">
					The <strong>change summary</strong> lists what happened in plain language: which nodes
					were renamed, moved, added, or modified. This is the fastest way to understand the diff at
					a glance.
				</p>
				<p className="help-text">
					<strong>Semantic verdict pills</strong> classify each top-level declaration. Click a pill
					to scroll both panes to the relevant code.
				</p>
				<p className="help-text">
					The <strong>diff view</strong> shows both versions side by side with syntax highlighting.
					Changed lines have a colored left-edge bar indicating the operation type. Within changed
					lines, <strong>word-level inline highlighting</strong> marks the specific tokens that
					differ.
				</p>
				<p className="help-text">
					<strong>Move arrows</strong> - orange curves between the panes - connect nodes that
					changed position. Hover over an arrow to highlight both the source and destination lines.
				</p>
				<p className="help-text">
					<strong>Collapsed regions</strong> hide unchanged code, showing 3 lines of context around
					each change. Click a collapsed region to expand it. The two panes scroll in sync. Large
					files are virtualized for smooth scrolling regardless of file size.
				</p>
			</section>

			<section className="help-section">
				<h2 className="help-heading">Permalinks and sharing</h2>
				<p className="help-text">
					Every diff result is stored server-side and gets a unique URL. You can bookmark, share, or
					revisit any diff result by its URL. The original and modified source code are stored
					alongside the result, so the page renders fully even without re-submitting.
				</p>
				<p className="help-text">
					Use <strong>Copy link</strong> to copy the permalink URL, or <strong>Copy summary</strong>{" "}
					to get a formatted text summary with operation counts, change descriptions, and the URL -
					ready to paste into a code review or chat.
				</p>
			</section>

			<section className="help-section">
				<h2 className="help-heading">Git integration</h2>
				<p className="help-text">
					The <strong>Git diff</strong> tab lets you diff any file between two git refs in a local
					repository. Stratum runs <code>git show</code> to extract file contents at each ref and{" "}
					<code>git diff --name-status</code> to list changed files.
				</p>
				<p className="help-text">
					Refs can be branch names (<code>main</code>, <code>feature-branch</code>), relative refs (
					<code>HEAD~1</code>, <code>HEAD~5</code>), commit hashes, or tags. The language is
					detected automatically from the file extension. Files with unrecognized extensions fall
					back to line-level comparison.
				</p>
			</section>

			<section className="help-section">
				<h2 className="help-heading">Operation types</h2>
				<div className="help-ops">
					{operations.map((op) => (
						<div key={op.name} className="help-op">
							<div className="help-op-header">
								<span className="help-op-dot" style={{ background: op.color }} />
								<span className="help-op-name">{op.name}</span>
							</div>
							<p className="help-op-desc">{op.description}</p>
							<div
								className="help-op-sample"
								style={{
									background: op.bg,
									borderLeft: `2px solid ${op.color}`,
								}}
							>
								<span className="help-mono" style={{ color: "var(--foreground-muted)" }}>
									example code line
								</span>
							</div>
						</div>
					))}
				</div>
			</section>

			<section className="help-section">
				<h2 className="help-heading">Semantic verdicts</h2>
				<p className="help-text">
					Stratum classifies matched functions with conservative edit-shape heuristics. For Go,
					direct statement reorderings receive an additional bounded def/use analysis. These
					verdicts are evidence for review, not semantic proof, and can be wrong.
				</p>
				<dl className="help-verdicts">
					<div className="help-verdict-row">
						<dt className="help-verdict-term">behavior-preserving</dt>
						<dd className="help-verdict-desc">
							The node was moved, reformatted, or its surrounding context changed, but the logic
							within it appears identical. This is a heuristic verdict based on edit-script shape,
							not semantic proof -- review the body if in doubt.
						</dd>
					</div>
					<div className="help-verdict-row">
						<dt className="help-verdict-term">behavior-changing</dt>
						<dd className="help-verdict-desc">
							The node's body, parameters, return type, or internal logic changed. The reason (e.g.,
							&ldquo;body edited&rdquo;, &ldquo;signature changed&rdquo;) is shown alongside the
							verdict.
						</dd>
					</div>
					<div className="help-verdict-row">
						<dt className="help-verdict-term">indeterminate</dt>
						<dd className="help-verdict-desc">
							The tool cannot confidently classify the change. This includes calls, control flow,
							nested reorderings, large blocks, and other cases where side effects cannot be ruled
							out.
						</dd>
					</div>
				</dl>
			</section>

			<section className="help-section">
				<h2 className="help-heading">The algorithm</h2>
				<p className="help-text">
					Stratum's matching algorithm runs in six stages. Understanding these helps interpret edge
					cases in the output.
				</p>
				<ol className="help-algorithm">
					{algorithmSteps.map((step) => (
						<li key={step.name}>
							<strong>{step.name}.</strong> {step.description}
						</li>
					))}
				</ol>
			</section>

			<section className="help-section">
				<h2 className="help-heading">Language support</h2>
				<p className="help-text">
					Stratum's parser architecture is pluggable. Each language has a parser that produces a
					structural tree. The richer the tree, the better the diff quality.
				</p>
				<table className="help-table">
					<thead>
						<tr>
							<th>Language</th>
							<th>Parser</th>
							<th>Level</th>
							<th>What it detects</th>
						</tr>
					</thead>
					<tbody>
						{languages.map((lang) => (
							<tr key={lang.name}>
								<td>{lang.name}</td>
								<td className="help-mono">{lang.parser}</td>
								<td>{lang.level}</td>
								<td className="help-text-small">{lang.detail}</td>
							</tr>
						))}
					</tbody>
				</table>
				<p className="help-text help-note">
					<strong>Full AST</strong> parsers produce a complete syntax tree - every statement and
					expression is a node. <strong>Structural</strong> parsers identify major declarations
					(functions, classes, methods) for accurate move and rename detection, while treating the
					code within them as text. <strong>Line diff</strong> falls back to line-by-line comparison
					with trivial-line filtering (empty lines, lone braces, and punctuation-only lines are
					excluded to reduce false-positive moves). Additional language support is driven by demand.
				</p>
			</section>

			<section className="help-section">
				<h2 className="help-heading">Keyboard shortcuts</h2>
				<dl className="help-verdicts">
					<div className="help-verdict-row">
						<dt className="help-verdict-term">Ctrl+Enter</dt>
						<dd className="help-verdict-desc">Submit the diff (paste mode).</dd>
					</div>
				</dl>
			</section>

			<section className="help-section">
				<h2 className="help-heading">Bounded computation</h2>
				<p className="help-text">
					The optimal alignment stage has a node budget. When an unmatched region is too large, the
					algorithm skips expensive alignment, reports the remaining nodes as insertions and
					deletions, and marks the region &ldquo;approximate.&rdquo; This bounds work on
					pathological inputs while preserving structural analysis where feasible.
				</p>
				<p className="help-text">
					Approximate regions are marked in the diff view with a{" "}
					<strong>dashed left-edge border</strong> and a subtle background tint. The result header
					also shows the count of approximate regions. You always get a result, and you always know
					when the result is imperfect.
				</p>
			</section>

			<section className="help-section">
				<h2 className="help-heading">Caching</h2>
				<p className="help-text">
					Stratum uses content-addressed caching. Parse results are cached by the SHA-256 hash of
					the input source, and diff results are cached by the pair of source hashes. Identical
					inputs never recompute - submitting the same code pair returns the cached result
					instantly.
				</p>
			</section>

			<section className="help-section">
				<h2 className="help-heading">Performance</h2>
				<p className="help-text">
					The matching algorithm is designed for interactive latency. Typical diffs (functions and
					classes under a few hundred nodes) complete in under a millisecond. A 100-function file
					diffs in approximately 12ms end-to-end including parsing. The three-phase matching
					strategy means most work is O(n) via hash comparison, with the expensive optimal alignment
					running only on small, ambiguous subtrees.
				</p>
			</section>

			<section className="help-section">
				<h2 className="help-heading">Roadmap</h2>
				<p className="help-text" style={{ marginBottom: 12 }}>
					<strong>v2</strong> - Semantic evidence and evaluation
				</p>
				<dl className="help-verdicts">
					<div className="help-verdict-row">
						<dt className="help-verdict-term">Dataflow analysis</dt>
						<dd className="help-verdict-desc">
							Implemented for direct Go statement reorderings. Independent def/use sets are
							classified as preserving, dependencies as changing, and uncertain side effects as
							indeterminate.
						</dd>
					</div>
					<div className="help-verdict-row">
						<dt className="help-verdict-term">Public benchmark</dt>
						<dd className="help-verdict-desc">
							Remaining: a labeled corpus from real open-source history with published
							move-detection precision, recall, methodology, and reproducible results.
						</dd>
					</div>
				</dl>
				<p className="help-text" style={{ marginTop: 16, marginBottom: 12 }}>
					<strong>v3 (current)</strong> - Structural merge and cross-file analysis
				</p>
				<dl className="help-verdicts">
					<div className="help-verdict-row">
						<dt className="help-verdict-term">Three-way merge</dt>
						<dd className="help-verdict-desc">
							Implemented. The Merge tab accepts a base, left, and right source and produces a
							per-node merge plan with structural conflict classification: modify-modify,
							delete-modify, rename-rename, and add-add. Each structural unit shows a verdict and
							the chosen resolution.
						</dd>
					</div>
					<div className="help-verdict-row">
						<dt className="help-verdict-term">Cross-file detection</dt>
						<dd className="help-verdict-desc">
							Implemented. In the Git diff tab, &quot;Analyze changeset&quot; diffs all changed
							files between two refs and detects cross-file moves and rename-moves. Uses content
							hashing for exact moves and line-set Jaccard similarity for edited moves (threshold
							0.6) and rename-moves (threshold 0.75).
						</dd>
					</div>
				</dl>
				<p className="help-text" style={{ marginTop: 16, marginBottom: 12 }}>
					<strong>Remaining</strong>
				</p>
				<dl className="help-verdicts">
					<div className="help-verdict-row">
						<dt className="help-verdict-term">Public benchmark</dt>
						<dd className="help-verdict-desc">
							A labeled corpus from real open-source history with published move-detection
							precision, recall, methodology, and reproducible results.
						</dd>
					</div>
					<div className="help-verdict-row">
						<dt className="help-verdict-term">Language plugins</dt>
						<dd className="help-verdict-desc">
							Additional language parsers and deeper fidelity for existing ones, driven by demand.
						</dd>
					</div>
				</dl>
			</section>
		</div>
	);
}
