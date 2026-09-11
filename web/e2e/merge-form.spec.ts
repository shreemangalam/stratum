import { expect, test } from "@playwright/test";

const goBase = `package main

func add(a, b int) int {
	return a + b
}

func sub(a, b int) int {
	return a - b
}`;

const goLeft = `package main

func add(a, b int) int {
	return a + b
}

func sub(a, b int) int {
	return a - b
}

func mul(a, b int) int {
	return a * b
}`;

const goRight = `package main

func add(a, b int) int {
	sum := a + b
	return sum
}

func sub(a, b int) int {
	return a - b
}`;

const conflictLeft = `package main

func compute(x int) int {
	return x * 2
}`;

const conflictRight = `package main

func compute(x int) int {
	return x * 3
}`;

const conflictBase = `package main

func compute(x int) int {
	return x + 1
}`;

function mockMergeRoute(
	page: import("@playwright/test").Page,
	response: object,
	status = 200,
) {
	return page.route("**/api/v1/merges", async (route) => {
		if (route.request().method() !== "POST") {
			await route.continue();
			return;
		}
		await route.fulfill({
			status,
			contentType: "application/json",
			body: JSON.stringify(response),
		});
	});
}

test.describe("Merge tab UI", () => {
	test("switches to merge tab and shows three text areas", async ({ page }) => {
		await page.goto("/");
		const mergeTab = page.locator("button.tab-btn", { hasText: "Merge" });
		await mergeTab.click();
		await expect(mergeTab).toHaveClass(/active/);

		const textareas = page.locator("textarea");
		await expect(textareas).toHaveCount(3);

		await expect(textareas.nth(0)).toHaveAttribute("placeholder", /ancestor|base/i);
		await expect(textareas.nth(1)).toHaveAttribute("placeholder", /left/i);
		await expect(textareas.nth(2)).toHaveAttribute("placeholder", /right/i);
	});

	test("merge button is disabled when inputs are empty", async ({ page }) => {
		await page.goto("/");
		await page.locator("button.tab-btn", { hasText: "Merge" }).click();

		const mergeButton = page.locator("button.btn-primary", { hasText: "Merge" });
		await expect(mergeButton).toBeDisabled();
	});

	test("merge button enables when all three inputs are filled", async ({ page }) => {
		await page.goto("/");
		await page.locator("button.tab-btn", { hasText: "Merge" }).click();

		const textareas = page.locator("textarea");
		await textareas.nth(0).fill(goBase);
		await textareas.nth(1).fill(goLeft);
		await textareas.nth(2).fill(goRight);

		const mergeButton = page.locator("button.btn-primary", { hasText: "Merge" });
		await expect(mergeButton).toBeEnabled();
	});

	test("shows planning state on submit", async ({ page }) => {
		await page.route("**/api/v1/merges", async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 1000));
			await route.fulfill({
				status: 200,
				contentType: "application/json",
				body: JSON.stringify({
					plan: { entries: [], conflict_count: 0 },
				}),
			});
		});

		await page.goto("/");
		await page.locator("button.tab-btn", { hasText: "Merge" }).click();

		const textareas = page.locator("textarea");
		await textareas.nth(0).fill(goBase);
		await textareas.nth(1).fill(goLeft);
		await textareas.nth(2).fill(goRight);

		const mergeButton = page.locator("button.btn-primary");
		await mergeButton.click();
		await expect(mergeButton).toHaveText("Planning...");
		await expect(mergeButton).toBeDisabled();
	});

	test("renders auto-resolved merge result", async ({ page }) => {
		await mockMergeRoute(page, {
			plan: {
				entries: [
					{
						base_node: { kind: "function_declaration", label: "add" },
						left_node: { kind: "function_declaration", label: "add" },
						right_node: { kind: "function_declaration", label: "add" },
						decision: "take-right",
						reason: "modified on right only",
					},
					{
						base_node: { kind: "function_declaration", label: "sub" },
						left_node: { kind: "function_declaration", label: "sub" },
						right_node: { kind: "function_declaration", label: "sub" },
						decision: "unchanged",
						reason: "identical in both",
					},
					{
						base_node: null,
						left_node: { kind: "function_declaration", label: "mul" },
						right_node: null,
						decision: "take-left",
						reason: "added on left only",
					},
				],
				conflict_count: 0,
			},
		});

		await page.goto("/");
		await page.locator("button.tab-btn", { hasText: "Merge" }).click();

		const textareas = page.locator("textarea");
		await textareas.nth(0).fill(goBase);
		await textareas.nth(1).fill(goLeft);
		await textareas.nth(2).fill(goRight);

		await page.locator("button.btn-primary", { hasText: "Merge" }).click();

		const result = page.locator(".merge-result");
		await expect(result).toBeVisible({ timeout: 5000 });

		await expect(page.locator(".merge-summary")).toContainText("3");
		await expect(page.locator(".merge-summary-auto")).toContainText("3");

		const rows = page.locator(".merge-table tbody tr");
		await expect(rows).toHaveCount(3);
		await expect(rows.nth(0)).toContainText("add");
		await expect(rows.nth(0)).toContainText("Take right");
		await expect(rows.nth(1)).toContainText("sub");
		await expect(rows.nth(1)).toContainText("Unchanged");
		await expect(rows.nth(2)).toContainText("mul");
		await expect(rows.nth(2)).toContainText("Take left");
	});

	test("renders merge result with conflicts", async ({ page }) => {
		await mockMergeRoute(page, {
			plan: {
				entries: [
					{
						base_node: { kind: "function_declaration", label: "compute" },
						left_node: { kind: "function_declaration", label: "compute" },
						right_node: { kind: "function_declaration", label: "compute" },
						decision: "conflict",
						conflict_kind: "modify-modify",
						reason: "both sides modified",
					},
				],
				conflict_count: 1,
			},
		});

		await page.goto("/");
		await page.locator("button.tab-btn", { hasText: "Merge" }).click();

		const textareas = page.locator("textarea");
		await textareas.nth(0).fill(conflictBase);
		await textareas.nth(1).fill(conflictLeft);
		await textareas.nth(2).fill(conflictRight);

		await page.locator("button.btn-primary", { hasText: "Merge" }).click();

		const result = page.locator(".merge-result");
		await expect(result).toBeVisible({ timeout: 5000 });

		await expect(page.locator(".merge-summary-conflicts")).toContainText("1");
		await expect(page.locator(".merge-summary-conflicts")).toContainText("conflict");

		const conflictRow = page.locator("tr.merge-conflict");
		await expect(conflictRow).toBeVisible();
		await expect(conflictRow).toContainText("Conflict");
		await expect(conflictRow).toContainText("modify-modify");
	});

	test("shows error when API returns failure", async ({ page }) => {
		await mockMergeRoute(
			page,
			{ error: "language not supported" },
			400,
		);

		await page.goto("/");
		await page.locator("button.tab-btn", { hasText: "Merge" }).click();

		const textareas = page.locator("textarea");
		await textareas.nth(0).fill("some base code");
		await textareas.nth(1).fill("some left code");
		await textareas.nth(2).fill("some right code");

		await page.locator("select[aria-label='Language']").selectOption("go");
		await page.locator("button.btn-primary", { hasText: "Merge" }).click();

		const errorText = page.locator(".error-text");
		await expect(errorText).toBeVisible({ timeout: 5000 });
		await expect(errorText).toContainText("language not supported");
	});

	test("shows validation error when language cannot be detected", async ({ page }) => {
		await page.goto("/");
		await page.locator("button.tab-btn", { hasText: "Merge" }).click();

		const textareas = page.locator("textarea");
		await textareas.nth(0).fill("aaa bbb ccc");
		await textareas.nth(1).fill("ddd eee fff");
		await textareas.nth(2).fill("ggg hhh iii");

		await page.locator("button.btn-primary", { hasText: "Merge" }).click();

		const errorText = page.locator(".error-text");
		await expect(errorText).toBeVisible({ timeout: 5000 });
		await expect(errorText).toContainText(/language/i);
	});

	test("has a language selector with auto-detect default", async ({ page }) => {
		await page.goto("/");
		await page.locator("button.tab-btn", { hasText: "Merge" }).click();

		const langSelect = page.locator("select[aria-label='Language']");
		await expect(langSelect).toBeVisible();
		await expect(langSelect).toHaveValue("auto");
	});

	test("auto-detects Go language from content", async ({ page }) => {
		await page.goto("/");
		await page.locator("button.tab-btn", { hasText: "Merge" }).click();

		const textareas = page.locator("textarea");
		await textareas.nth(0).fill(goBase);

		await expect(page.locator(".status-text")).toContainText("Go", { timeout: 3000 });
	});

	test("clears result when input changes", async ({ page }) => {
		await mockMergeRoute(page, {
			plan: {
				entries: [
					{
						base_node: { kind: "function_declaration", label: "f" },
						decision: "unchanged",
						reason: "no change",
					},
				],
				conflict_count: 0,
			},
		});

		await page.goto("/");
		await page.locator("button.tab-btn", { hasText: "Merge" }).click();

		const textareas = page.locator("textarea");
		await textareas.nth(0).fill(goBase);
		await textareas.nth(1).fill(goLeft);
		await textareas.nth(2).fill(goRight);

		await page.locator("button.btn-primary", { hasText: "Merge" }).click();
		await expect(page.locator(".merge-result")).toBeVisible({ timeout: 5000 });

		await textareas.nth(0).fill("package main\n\nfunc changed() {}");
		await expect(page.locator(".merge-result")).toBeHidden();
	});
});
