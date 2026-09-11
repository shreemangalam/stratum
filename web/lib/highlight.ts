import { type BundledLanguage, createHighlighter, type Highlighter } from "shiki";

export interface HighlightToken {
	content: string;
	dark: string;
	light: string;
}

const LANG_MAP: Record<string, BundledLanguage> = {
	go: "go",
	javascript: "javascript",
	typescript: "typescript",
	python: "python",
	java: "java",
	c: "c",
	cpp: "cpp",
	xslt: "xml",
};

const SUPPORTED_LANGS = [...new Set(Object.values(LANG_MAP))];

let highlighterPromise: Promise<Highlighter> | null = null;

function getHighlighter(): Promise<Highlighter> {
	if (!highlighterPromise) {
		highlighterPromise = createHighlighter({
			themes: ["github-dark-default", "github-light-default"],
			langs: SUPPORTED_LANGS,
		});
	}
	return highlighterPromise;
}

export async function tokenizeLines(code: string, language: string): Promise<HighlightToken[][]> {
	const lang = LANG_MAP[language];
	if (!lang) {
		return code.split("\n").map((line) => [{ content: line, dark: "", light: "" }]);
	}

	const highlighter = await getHighlighter();
	const dark = highlighter.codeToTokens(code, { lang, theme: "github-dark-default" });
	const light = highlighter.codeToTokens(code, { lang, theme: "github-light-default" });

	return dark.tokens.map((darkLine, i) => {
		const lightLine = light.tokens[i] ?? [];
		return darkLine.map((t, j) => ({
			content: t.content,
			dark: t.color ?? "",
			light: lightLine[j]?.color ?? t.color ?? "",
		}));
	});
}
