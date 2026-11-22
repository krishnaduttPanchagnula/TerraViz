import { test, expect } from '@playwright/test';

// The server is started with samples/network.tfstate ingested into SQLite.
test('new dashboard loads with the ingested state', async ({ page }) => {
  await page.goto('/');
  await expect(page).toHaveTitle(/terraquery|terraviz/i);
  await expect(page.locator('#statStates')).toHaveText('1', { timeout: 15_000 });
  await expect(page.locator('#graph')).toBeVisible();
  await expect(page.locator('link[rel="icon"]')).toHaveAttribute('href', 'icon.svg');
  await expect(page.locator('.brand img')).toHaveAttribute('src', 'icon.svg');
});

test('app icon is served as an SVG', async ({ request }) => {
  const res = await request.get('/icon.svg');
  expect(res.ok()).toBeTruthy();
  expect(res.headers()['content-type']).toContain('image/svg+xml');
  expect(await res.text()).toContain('<svg');
});

test('states API returns the ingested state', async ({ request }) => {
  const res = await request.get('/api/states');
  expect(res.ok()).toBeTruthy();
  const body = await res.json();
  expect(body.states).toHaveLength(1);
  expect(body.states[0].name).toBe('acme-network');
});

test('graph API exposes resources and dependency links', async ({ request }) => {
  const resources = await (await request.get('/api/resources')).json();
  expect(resources.resources.length).toBeGreaterThan(0);
  const graph = await (await request.get('/api/graph?min=0.5')).json();
  expect(graph.nodes).toBeTruthy();
});
