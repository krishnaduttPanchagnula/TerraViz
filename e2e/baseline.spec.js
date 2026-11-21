import { test, expect } from '@playwright/test';

// The fixture is a ScanResult with 10 resources and 8 connections.
test('app loads the diagram and renders nodes', async ({ page }) => {
  await page.goto('/terraviz/');
  await expect(page).toHaveTitle(/Cloud Architecture Visualizer/);
  await expect(page.locator('.loading-overlay')).toHaveClass(/hidden/, { timeout: 15_000 });
  await expect(page.locator('.resource-node').first()).toBeVisible({ timeout: 15_000 });
  const nodes = await page.locator('.resource-node').count();
  expect(nodes).toBeGreaterThan(0);
});

test('diagram API returns the fixture', async ({ request }) => {
  const res = await request.get('/terraviz/api/diagram');
  expect(res.ok()).toBeTruthy();
  const body = await res.json();
  expect(body.diagram.resources).toHaveLength(10);
  expect(body.diagram.connections).toHaveLength(8);
});

test('referenced-missing API returns a list', async ({ request }) => {
  const res = await request.get('/terraviz/api/missing');
  expect(res.ok()).toBeTruthy();
  expect(Array.isArray(await res.json())).toBeTruthy();
});
