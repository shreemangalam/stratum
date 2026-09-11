import { expect, test } from "@playwright/test";

test.describe("Full-stack diff", () => {
	test.skip(
		process.env.STRATUM_E2E_FULL_STACK !== "1",
		"requires the API, worker, and Postgres stack",
	);

	test("submits and renders a dataflow-aware semantic result", async ({ page }) => {
		await page.goto("/");

		const textareas = page.locator("textarea");
		await textareas.first().fill(`package main
func f() int {
	x := 31
	y := x + 41
	return y
}`);
		await textareas.last().fill(`package main
func f() int {
	y := x + 41
	x := 31
	return y
}`);

		await page.getByRole("button", { name: "Diff", exact: true }).click();
		await expect(page).toHaveURL(/\/diffs\/[0-9a-f-]+$/, { timeout: 15000 });
		await expect(page.locator(".result-header")).toBeVisible({ timeout: 20000 });
		await expect(page.locator(".verdict-pill")).toContainText(
			"behavior-changing - dependent statements reordered",
		);
		await expect(page.locator(".change-summary")).toContainText("dependent statements reordered");
	});
});
