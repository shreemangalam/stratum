import { expect, test } from "@playwright/test";

test.describe("Diff form interaction", () => {
	test("can type into both text areas", async ({ page }) => {
		await page.goto("/");
		const textareas = page.locator("textarea");
		await textareas.first().fill("func hello() {}");
		await textareas.last().fill("func world() {}");
		await expect(textareas.first()).toHaveValue("func hello() {}");
		await expect(textareas.last()).toHaveValue("func world() {}");
	});

	test("diff button shows submitting state on click", async ({ page }) => {
		await page.route("**/api/v1/diffs", async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 1000));
			await route.fulfill({
				status: 201,
				contentType: "application/json",
				body: JSON.stringify({ id: "test-job", status: "pending" }),
			});
		});
		await page.goto("/");
		const textareas = page.locator("textarea");
		await textareas.first().fill("package main\n\nfunc a() {}");
		await textareas.last().fill("package main\n\nfunc b() {}");

		const diffButton = page.locator("button.btn-primary");
		await diffButton.click();
		await expect(diffButton).toHaveText("Submitting...");
		await expect(diffButton).toBeDisabled();
	});

	test("language selector opens and has options", async ({ page }) => {
		await page.goto("/");
		const langSelect = page.locator("[aria-label='Language']");
		await langSelect.click();
		// Options should appear in the dropdown
		const options = page.locator("[role='option']");
		await expect(options.first()).toBeVisible({ timeout: 3000 });
		const count = await options.count();
		expect(count).toBeGreaterThan(3);
	});

	test("API retry button works", async ({ page }) => {
		let healthRequests = 0;
		let allowHealthyResponse = false;
		await page.route("**/api/v1/health", async (route) => {
			healthRequests++;
			if (!allowHealthyResponse) {
				await route.abort("connectionrefused");
				return;
			}
			await route.fulfill({
				status: 200,
				contentType: "application/json",
				body: JSON.stringify({ status: "ok", database: "connected" }),
			});
		});
		await page.goto("/");
		const banner = page.locator(".api-banner");
		await expect(banner).toBeVisible({ timeout: 10000 });
		const retryButton = banner.locator("button", { hasText: /retry/i });
		await expect(retryButton).toBeVisible();
		allowHealthyResponse = true;
		await retryButton.click();
		await expect(banner).toBeHidden({ timeout: 10000 });
		expect(healthRequests).toBeGreaterThanOrEqual(2);
	});
});
