export interface DiffSpan {
	text: string;
	type: "equal" | "insert" | "delete";
}

export function inlineDiff(oldText: string, newText: string): DiffSpan[] {
	const oldTokens = tokenize(oldText);
	const newTokens = tokenize(newText);
	const lcs = longestCommonSubsequence(oldTokens, newTokens);

	const spans: DiffSpan[] = [];
	let oi = 0;
	let ni = 0;
	let li = 0;

	while (oi < oldTokens.length || ni < newTokens.length) {
		if (li < lcs.length) {
			const anchor = lcs[li];
			while (oi < oldTokens.length && oldTokens[oi] !== anchor) {
				const tok = oldTokens[oi];
				if (tok !== undefined) pushSpan(spans, tok, "delete");
				oi++;
			}
			while (ni < newTokens.length && newTokens[ni] !== anchor) {
				const tok = newTokens[ni];
				if (tok !== undefined) pushSpan(spans, tok, "insert");
				ni++;
			}
			if (anchor !== undefined) {
				pushSpan(spans, anchor, "equal");
				oi++;
				ni++;
				li++;
			}
		} else {
			while (oi < oldTokens.length) {
				const tok = oldTokens[oi];
				if (tok !== undefined) pushSpan(spans, tok, "delete");
				oi++;
			}
			while (ni < newTokens.length) {
				const tok = newTokens[ni];
				if (tok !== undefined) pushSpan(spans, tok, "insert");
				ni++;
			}
		}
	}

	return mergeSpans(spans);
}

function pushSpan(spans: DiffSpan[], text: string, type: DiffSpan["type"]): void {
	const last = spans[spans.length - 1];
	if (last && last.type === type) {
		last.text += text;
	} else {
		spans.push({ text, type });
	}
}

function mergeSpans(spans: DiffSpan[]): DiffSpan[] {
	return spans.filter((s) => s.text.length > 0);
}

function tokenize(text: string): string[] {
	const tokens: string[] = [];
	let i = 0;
	while (i < text.length) {
		const ch = text[i];
		if (ch === undefined) break;
		if (/\s/.test(ch)) {
			let j = i;
			while (j < text.length && /\s/.test(text[j] ?? "")) j++;
			tokens.push(text.slice(i, j));
			i = j;
		} else if (/\w/.test(ch)) {
			let j = i;
			while (j < text.length && /\w/.test(text[j] ?? "")) j++;
			tokens.push(text.slice(i, j));
			i = j;
		} else {
			tokens.push(ch);
			i++;
		}
	}
	return tokens;
}

function longestCommonSubsequence(a: string[], b: string[]): string[] {
	const m = a.length;
	const n = b.length;

	if (m > 200 || n > 200) {
		return greedyLCS(a, b);
	}

	const dp: number[][] = Array.from({ length: m + 1 }, () =>
		Array.from({ length: n + 1 }, () => 0),
	);

	for (let i = 1; i <= m; i++) {
		const row = dp[i];
		const prevRow = dp[i - 1];
		if (!row || !prevRow) continue;
		for (let j = 1; j <= n; j++) {
			if (a[i - 1] === b[j - 1]) {
				row[j] = (prevRow[j - 1] ?? 0) + 1;
			} else {
				row[j] = Math.max(prevRow[j] ?? 0, row[j - 1] ?? 0);
			}
		}
	}

	const result: string[] = [];
	let i = m;
	let j = n;
	while (i > 0 && j > 0) {
		if (a[i - 1] === b[j - 1]) {
			const val = a[i - 1];
			if (val !== undefined) result.push(val);
			i--;
			j--;
		} else if ((dp[i - 1]?.[j] ?? 0) > (dp[i]?.[j - 1] ?? 0)) {
			i--;
		} else {
			j--;
		}
	}

	return result.reverse();
}

function greedyLCS(a: string[], b: string[]): string[] {
	const bIndex = new Map<string, number[]>();
	for (let j = 0; j < b.length; j++) {
		const token = b[j];
		if (token === undefined) continue;
		let positions = bIndex.get(token);
		if (!positions) {
			positions = [];
			bIndex.set(token, positions);
		}
		positions.push(j);
	}

	const result: string[] = [];
	let lastJ = -1;
	for (let i = 0; i < a.length; i++) {
		const token = a[i];
		if (token === undefined) continue;
		const positions = bIndex.get(token);
		if (!positions) continue;
		for (const j of positions) {
			if (j > lastJ) {
				result.push(token);
				lastJ = j;
				break;
			}
		}
	}
	return result;
}
