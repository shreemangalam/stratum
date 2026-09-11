"use client";

import { useRouter } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";
import { type ChangedFile, createGitDiff, getGitFiles } from "@/lib/api/client";

const STATUS_COLORS: Record<string, string> = {
	added: "var(--diff-add)",
	deleted: "var(--diff-remove)",
	modified: "var(--diff-change)",
	renamed: "var(--diff-move)",
};

const STATUS_LABELS: Record<string, string> = {
	added: "A",
	deleted: "D",
	modified: "M",
	renamed: "R",
};

interface FolderNode {
	name: string;
	path: string;
	files: ChangedFile[];
	children: FolderNode[];
}

function buildFileTree(files: ChangedFile[]): FolderNode {
	const root: FolderNode = { name: "", path: "", files: [], children: [] };

	for (const file of files) {
		const parts = file.path.split("/");
		let current = root;

		for (let i = 0; i < parts.length - 1; i++) {
			const dirName = parts[i];
			if (dirName === undefined) continue;
			const dirPath = parts.slice(0, i + 1).join("/");
			let child = current.children.find((c) => c.name === dirName);
			if (!child) {
				child = { name: dirName, path: dirPath, files: [], children: [] };
				current.children.push(child);
			}
			current = child;
		}
		current.files.push(file);
	}

	return root;
}

function flattenSingleChildDirs(node: FolderNode): FolderNode {
	if (node.children.length === 1 && node.files.length === 0) {
		const child = node.children[0];
		if (!child) return node;
		const merged: FolderNode = {
			name: node.name ? `${node.name}/${child.name}` : child.name,
			path: child.path,
			files: child.files,
			children: child.children,
		};
		return flattenSingleChildDirs(merged);
	}
	return {
		...node,
		children: node.children.map(flattenSingleChildDirs),
	};
}

function FileTreeNode({
	node,
	selectedFile,
	onSelect,
	depth,
}: {
	node: FolderNode;
	selectedFile: string | null;
	onSelect: (path: string) => void;
	depth: number;
}) {
	const [expanded, setExpanded] = useState(true);

	const hasContent = node.files.length > 0 || node.children.length > 0;
	if (!hasContent) return null;

	return (
		<div>
			{node.name && (
				<button
					type="button"
					className="git-tree-folder"
					onClick={() => setExpanded((e) => !e)}
					style={{ paddingLeft: depth * 16 + 8 }}
				>
					<span className="git-tree-arrow">{expanded ? "\u25BE" : "\u25B8"}</span>
					<span className="git-tree-folder-icon">{expanded ? "\uD83D\uDCC2" : "\uD83D\uDCC1"}</span>
					<span className="git-tree-folder-name">{node.name}</span>
					<span className="git-tree-count">{countFiles(node)}</span>
				</button>
			)}
			{expanded && (
				<>
					{node.children.map((child) => (
						<FileTreeNode
							key={child.path}
							node={child}
							selectedFile={selectedFile}
							onSelect={onSelect}
							depth={node.name ? depth + 1 : depth}
						/>
					))}
					{node.files.map((f) => (
						<button
							key={f.path}
							type="button"
							className={`git-file-item${selectedFile === f.path ? " selected" : ""}`}
							onClick={() => onSelect(f.path)}
							style={{ paddingLeft: (node.name ? depth + 1 : depth) * 16 + 8 }}
						>
							<span
								className="git-file-status"
								style={{ color: STATUS_COLORS[f.status] ?? "var(--foreground-muted)" }}
							>
								{STATUS_LABELS[f.status] ?? f.status[0]?.toUpperCase()}
							</span>
							<span className="git-file-path">{f.path.split("/").pop()}</span>
						</button>
					))}
				</>
			)}
		</div>
	);
}

function countFiles(node: FolderNode): number {
	return node.files.length + node.children.reduce((sum, c) => sum + countFiles(c), 0);
}

export function GitDiffForm() {
	const router = useRouter();
	const [repoPath, setRepoPath] = useState("");
	const [leftRef, setLeftRef] = useState("HEAD~1");
	const [rightRef, setRightRef] = useState("HEAD");
	const [files, setFiles] = useState<ChangedFile[]>([]);
	const [selectedFile, setSelectedFile] = useState<string | null>(null);
	const [loading, setLoading] = useState(false);
	const [submitting, setSubmitting] = useState(false);
	const [error, setError] = useState<string | null>(null);
	const [hasLoaded, setHasLoaded] = useState(false);
	const repoRef = useRef<HTMLInputElement>(null);

	const fileTree = files.length > 0 ? flattenSingleChildDirs(buildFileTree(files)) : null;

	const loadFiles = useCallback(async () => {
		if (!repoPath.trim()) {
			setError("Repository path is required");
			return;
		}
		if (!leftRef.trim() || !rightRef.trim()) {
			setError("Both refs are required");
			return;
		}

		setLoading(true);
		setError(null);
		setFiles([]);
		setSelectedFile(null);
		setHasLoaded(false);

		try {
			const changed = await getGitFiles(repoPath.trim(), leftRef.trim(), rightRef.trim());
			setFiles(changed);
			setHasLoaded(true);
			if (changed.length === 1 && changed[0]) {
				setSelectedFile(changed[0].path);
			}
		} catch (e) {
			setError(e instanceof Error ? e.message : "Failed to list files");
		} finally {
			setLoading(false);
		}
	}, [repoPath, leftRef, rightRef]);

	const handleSubmit = useCallback(async () => {
		if (!repoPath.trim()) {
			setError("Repository path is required");
			return;
		}
		if (!selectedFile) {
			setError("Select a file to diff");
			return;
		}

		setSubmitting(true);
		setError(null);

		try {
			const job = await createGitDiff({
				repo_path: repoPath.trim(),
				file_path: selectedFile,
				left_ref: leftRef.trim(),
				right_ref: rightRef.trim(),
			});
			router.push(`/diffs/${job.id}`);
		} catch (e) {
			setError(e instanceof Error ? e.message : "Failed to create diff");
			setSubmitting(false);
		}
	}, [repoPath, selectedFile, leftRef, rightRef, router]);

	const handleKeyDown = useCallback(
		(e: React.KeyboardEvent) => {
			if (e.key === "Enter" && !e.shiftKey) {
				e.preventDefault();
				if (!hasLoaded) {
					loadFiles();
				} else if (selectedFile) {
					handleSubmit();
				}
			}
		},
		[hasLoaded, loadFiles, selectedFile, handleSubmit],
	);

	useEffect(() => {
		repoRef.current?.focus();
	}, []);

	const statusCounts = files.reduce<Record<string, number>>((acc, f) => {
		acc[f.status] = (acc[f.status] ?? 0) + 1;
		return acc;
	}, {});

	return (
		// biome-ignore lint/a11y/noStaticElementInteractions: keyboard shortcut wrapper
		<div onKeyDown={handleKeyDown}>
			<div className="git-form-grid">
				<div className="git-field">
					<label className="field-label" htmlFor="repo-path">
						Repository path
					</label>
					<input
						ref={repoRef}
						id="repo-path"
						className="git-input"
						type="text"
						value={repoPath}
						onChange={(e) => {
							setRepoPath(e.target.value);
							setHasLoaded(false);
						}}
						placeholder="/home/user/my-project or D:\\Projects\\my-repo"
						spellCheck={false}
					/>
				</div>
				<div className="git-ref-row">
					<div className="git-field">
						<label className="field-label" htmlFor="left-ref">
							From (base)
						</label>
						<input
							id="left-ref"
							className="git-input"
							type="text"
							value={leftRef}
							onChange={(e) => {
								setLeftRef(e.target.value);
								setHasLoaded(false);
							}}
							placeholder="HEAD~1, main, abc1234"
							spellCheck={false}
						/>
					</div>
					<div className="git-field">
						<label className="field-label" htmlFor="right-ref">
							To (target)
						</label>
						<input
							id="right-ref"
							className="git-input"
							type="text"
							value={rightRef}
							onChange={(e) => {
								setRightRef(e.target.value);
								setHasLoaded(false);
							}}
							placeholder="HEAD, feature-branch, def5678"
							spellCheck={false}
						/>
					</div>
					<button
						type="button"
						className="btn btn-primary"
						onClick={loadFiles}
						disabled={loading || !repoPath.trim() || !leftRef.trim() || !rightRef.trim()}
						style={{ alignSelf: "flex-end" }}
					>
						{loading ? "Loading..." : "List files"}
					</button>
				</div>
			</div>

			{error && (
				<div className="error-text" style={{ marginTop: 12 }}>
					{error}
				</div>
			)}

			{hasLoaded && files.length === 0 && !error && (
				<div className="git-empty-state">
					<span>
						No files changed between <code>{leftRef}</code> and <code>{rightRef}</code>
					</span>
				</div>
			)}

			{files.length > 0 && (
				<div className="git-files">
					<div className="git-files-header">
						<span className="field-label">
							{files.length} changed file{files.length !== 1 ? "s" : ""}
						</span>
						<span className="git-status-counts">
							{Object.entries(statusCounts).map(([status, count]) => (
								<span
									key={status}
									className="git-status-count"
									style={{ color: STATUS_COLORS[status] }}
								>
									{count}
									{STATUS_LABELS[status]}
								</span>
							))}
						</span>
					</div>
					<div className="git-file-list">
						{fileTree && (
							<FileTreeNode
								node={fileTree}
								selectedFile={selectedFile}
								onSelect={setSelectedFile}
								depth={0}
							/>
						)}
					</div>
					<div className="git-files-footer">
						{selectedFile && (
							<span className="git-selected-file" title={selectedFile}>
								{selectedFile}
							</span>
						)}
						<button
							type="button"
							className="btn btn-primary"
							onClick={handleSubmit}
							disabled={!selectedFile || submitting}
						>
							{submitting ? "Diffing..." : "Diff selected file"}
						</button>
					</div>
				</div>
			)}
		</div>
	);
}
