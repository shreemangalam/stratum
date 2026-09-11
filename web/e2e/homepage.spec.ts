import { expect, test } from "@playwright/test";

test.describe("Homepage", () => {
	test("renders with paste-code tab active by default", async ({ page }) => {
		await page.goto("/");
		const pasteTab = page.locator("button.tab-btn", { hasText: "Paste code" });
		await expect(pasteTab).toHaveClass(/active/);
		await expect(page.locator("textarea").first()).toBeVisible();
	});

	test("shows API unreachable banner when backend is down", async ({ page }) => {
		await page.route("**/api/v1/health", (route) => route.abort("connectionrefused"));
		await page.goto("/");
		const banner = page.locator(".api-banner");
		await expect(banner).toBeVisible({ timeout: 10000 });
		await expect(banner).toContainText("Cannot reach the API server");
	});

	test("has two text areas for original and modified code", async ({ page }) => {
		await page.goto("/");
		const textareas = page.locator("textarea");
		await expect(textareas).toHaveCount(2);
		await expect(textareas.first()).toHaveAttribute("placeholder", /original/i);
		await expect(textareas.last()).toHaveAttribute("placeholder", /modified/i);
	});

	test("has a language selector", async ({ page }) => {
		await page.goto("/");
		const langSelect = page.locator("[aria-label='Language']");
		await expect(langSelect).toBeVisible();
	});

	test("has a diff button", async ({ page }) => {
		await page.goto("/");
		const diffButton = page.locator("button.btn-primary", { hasText: "Diff" });
		await expect(diffButton).toBeVisible();
	});

	test("switches between paste and git tabs", async ({ page }) => {
		await page.goto("/");
		const gitTab = page.locator("button.tab-btn", { hasText: "Git diff" });
		await gitTab.click();
		await expect(gitTab).toHaveClass(/active/);

		const pasteTab = page.locator("button.tab-btn", { hasText: "Paste code" });
		await expect(pasteTab).not.toHaveClass(/active/);

		await pasteTab.click();
		await expect(pasteTab).toHaveClass(/active/);
		await expect(gitTab).not.toHaveClass(/active/);
	});
});
