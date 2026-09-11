import type { components } from "@/lib/api/generated/schema";

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export type DiffInput = components["schemas"]["DiffInput"];
export type CreateDiffRequest = components["schemas"]["CreateDiffRequest"];
export type Location = components["schemas"]["Location"];
export type NodeRef = components["schemas"]["NodeRef"];
export type Operation = components["schemas"]["Operation"];
export type OpKind = Operation["kind"];
export type Span = components["schemas"]["Span"];
export type ApproximateRegion = components["schemas"]["ApproximateRegion"];
export type SemanticChange = components["schemas"]["SemanticChange"];
export type Verdict = SemanticChange["verdict"];
export type EditScript = components["schemas"]["EditScript"];
export type DiffJob = components["schemas"]["DiffJob"];
export type JobStatus = DiffJob["status"];
export type LanguageInfo = components["schemas"]["LanguageInfo"];

async function safeFetch(url: string, init?: RequestInit): Promise<Response> {
	try {
		return await fetch(url, init);
	} catch (e) {
		if (e instanceof TypeError) {
			throw new Error("Cannot reach the server - make sure the API is running");
		}
		throw e;
	}
}

async function parseErrorBody(res: Response, fallback: string): Promise<string> {
	try {
		const body = await res.json();
		return body.error ?? fallback;
	} catch {
		return fallback;
	}
}

export async function createDiff(req: CreateDiffRequest): Promise<DiffJob> {
	const res = await safeFetch(`${API_BASE}/api/v1/diffs`, {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify(req),
	});
	if (!res.ok) {
		if (res.status === 413) throw new Error("Request too large - maximum 1 MB per request");
		throw new Error(await parseErrorBody(res, "Failed to create diff"));
	}
	return res.json();
}

export async function getDiff(id: string): Promise<DiffJob> {
	const res = await safeFetch(`${API_BASE}/api/v1/diffs/${encodeURIComponent(id)}`);
	if (!res.ok) {
		throw new Error(await parseErrorBody(res, "Failed to get diff"));
	}
	return res.json();
}

export async function getLanguages(): Promise<LanguageInfo[]> {
	const res = await safeFetch(`${API_BASE}/api/v1/languages`);
	if (!res.ok) {
		return [];
	}
	const data = await res.json();
	return data.languages ?? [];
}

export type GitDiffRequest = components["schemas"]["GitDiffRequest"];
export type ChangedFile = components["schemas"]["ChangedFile"];

export async function createGitDiff(req: GitDiffRequest): Promise<DiffJob> {
	const res = await safeFetch(`${API_BASE}/api/v1/diffs/git`, {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify(req),
	});
	if (!res.ok) {
		throw new Error(await parseErrorBody(res, "Failed to create git diff"));
	}
	return res.json();
}

export async function getGitFiles(
	repoPath: string,
	leftRef: string,
	rightRef: string,
): Promise<ChangedFile[]> {
	const res = await safeFetch(`${API_BASE}/api/v1/git/files`, {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify({ repo_path: repoPath, left_ref: leftRef, right_ref: rightRef }),
	});
	if (!res.ok) {
		throw new Error(await parseErrorBody(res, "Failed to list changed files"));
	}
	const data = await res.json();
	return data.files ?? [];
}

export type CreateMergeRequest = components["schemas"]["CreateMergeRequest"];
export type MergeResponse = components["schemas"]["MergeResponse"];
export type MergePlan = components["schemas"]["MergePlan"];
export type MergeEntry = components["schemas"]["MergeEntry"];
export type MergeDecision = MergeEntry["decision"];

export async function createMerge(req: CreateMergeRequest): Promise<MergeResponse> {
	const res = await safeFetch(`${API_BASE}/api/v1/merges`, {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify(req),
	});
	if (!res.ok) {
		if (res.status === 413) throw new Error("Request too large - maximum 1 MB per request");
		throw new Error(await parseErrorBody(res, "Failed to create merge plan"));
	}
	return res.json();
}

export function streamDiff(id: string, onEvent: (type: string, data: string) => void): () => void {
	let cancelled = false;

	async function pollFallback() {
		while (!cancelled) {
			await new Promise((r) => setTimeout(r, 500));
			if (cancelled) return;
			try {
				const job = await getDiff(id);
				if (job.status === "completed" && job.result) {
					onEvent("result", JSON.stringify(job.result));
					return;
				}
				if (job.status === "failed") {
					onEvent("error", job.error ?? "Diff failed");
					return;
				}
				onEvent("status", job.status);
			} catch {
				onEvent("error", "Lost connection");
				return;
			}
		}
	}

	const url = `${API_BASE}/api/v1/diffs/${encodeURIComponent(id)}/stream`;
	const eventSource = new EventSource(url);

	eventSource.addEventListener("status", (e) => {
		onEvent("status", (e as MessageEvent).data);
	});
	eventSource.addEventListener("progress", (e) => {
		onEvent("progress", (e as MessageEvent).data);
	});
	eventSource.addEventListener("result", (e) => {
		onEvent("result", (e as MessageEvent).data);
		eventSource.close();
	});
	eventSource.addEventListener("error", (e) => {
		const data = (e as MessageEvent).data;
		eventSource.close();
		if (data) {
			onEvent("error", data);
		} else {
			pollFallback();
		}
	});

	return () => {
		cancelled = true;
		eventSource.close();
	};
}
