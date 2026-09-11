import { expect, test } from "@playwright/test";

test.describe("Help page", () => {
	test("renders with heading and content", async ({ page }) => {
		await page.goto("/help");
		await expect(page.locator("h1")).toContainText(/stratum/i);
	});

	test("has navigation back to home", async ({ page }) => {
		await page.goto("/help");
		const homeLink = page.locator("a[href='/']");
		await expect(homeLink).toBeVisible();
	});

	test("describes supported languages", async ({ page }) => {
		await page.goto("/help");
		const content = page.locator("main, article, body");
		await expect(content.first()).toContainText(/go/i);
	});
});
