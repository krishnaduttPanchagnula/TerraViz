import { test, expect } from '@playwright/test';
import { spawn } from 'node:child_process';

test('offline summary endpoint returns a deterministic brief', async ({ request }) => {
  const res = await request.post('/terraviz/api/summary');
  expect(res.ok()).toBeTruthy();
  const body = await res.json();
  expect(body.source).toBe('local');
  expect(body.summary).toContain('Architecture summary');
  expect(body.summary).toContain('10 resources');
});

test('cost overlay returns totals and an honest disclaimer', async ({ request }) => {
  const res = await request.get('/terraviz/api/costs');
  expect(res.ok()).toBeTruthy();
  const body = await res.json();
  expect(body.currency).toBe('USD');
  expect(body.total_monthly_usd).toBeGreaterThan(0);
  expect(body.disclaimer).toContain('Not a bill');
  expect(Array.isArray(body.resources)).toBeTruthy();
});

test('draw.io export is served from the embedded exporter', async ({ request }) => {
  const res = await request.get('/terraviz/api/export/drawio');
  expect(res.ok()).toBeTruthy();
  expect(res.headers()['content-type']).toContain('application/xml');
  const text = await res.text();
  expect(text).toContain('<mxfile');
  expect(text).toContain('<mxGraphModel');
});

test('local snapshot lifecycle: save, list, load, delete', async ({ request }) => {
  const saved = await request.post('/terraviz/api/snapshots', { data: { name: 'e2e-snapshot' } });
  expect(saved.ok()).toBeTruthy();
  const snap = await saved.json();
  expect(snap.name).toBe('e2e-snapshot');
  expect(snap.resources).toBe(10);

  const list = await request.get('/terraviz/api/snapshots');
  expect((await list.json()).some((s) => s.id === snap.id)).toBeTruthy();

  const load = await request.get(`/terraviz/api/snapshots/${snap.id}`);
  expect(load.ok()).toBeTruthy();
  expect((await load.json()).success).toBeTruthy();

  const del = await request.delete(`/terraviz/api/snapshots/${snap.id}`);
  expect(del.ok()).toBeTruthy();
  const after = await request.get('/terraviz/api/snapshots');
  expect((await after.json()).some((s) => s.id === snap.id)).toBeFalsy();
});

test('share link payload can be loaded back into the server', async ({ request }) => {
  const diagram = await (await request.get('/terraviz/api/diagram')).json();
  const res = await request.post('/terraviz/api/share/load', { data: diagram });
  expect(res.ok()).toBeTruthy();
  expect((await res.json()).success).toBeTruthy();
  const after = await (await request.get('/terraviz/api/diagram')).json();
  expect(after.diagram.resources).toHaveLength(10);
});

test('MCP stdio server answers initialize and exposes tools', async () => {
  const child = spawn('./terraviz', ['mcp', 'diagram_small.json'], {
    cwd: process.cwd(),
    stdio: ['pipe', 'pipe', 'pipe'],
  });
  let stdout = '';
  child.stdout.on('data', (chunk) => { stdout += chunk.toString(); });
  child.stdin.write(`${JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'initialize', params: {} })}\n`);
  child.stdin.write(`${JSON.stringify({ jsonrpc: '2.0', id: 2, method: 'tools/list' })}\n`);
  child.stdin.end();

  const code = await new Promise((resolve) => child.on('close', resolve));
  expect(code).toBe(0);
  const lines = stdout.trim().split('\n').filter(Boolean).map(JSON.parse);
  expect(lines[0].result.protocolVersion).toBeTruthy();
  expect(lines[1].result.tools.length).toBe(7);
});

test('insights panel renders summary, costs and a share URL', async ({ page }) => {
  await page.goto('/terraviz/');
  await expect(page.locator('.loading-overlay')).toHaveClass(/hidden/);

  await page.locator('#toggle-insights').click();
  await expect(page.locator('#insights-panel')).toHaveClass(/open/);

  await page.locator('#gen-summary').click();
  await expect(page.locator('#summary-text')).toContainText('Architecture summary');

  await page.locator('#gen-costs').click();
  await expect(page.locator('#cost-content')).toContainText('Estimated total:');

  await page.locator('#make-share').click();
  await expect(page.locator('#share-url')).toHaveValue(/#d=/);

  await page.locator('#close-insights').click();
  await expect(page.locator('#insights-panel')).not.toHaveClass(/open/);
});

test('draw.io export button downloads an editable file', async ({ page }) => {
  await page.goto('/terraviz/');
  await expect(page.locator('.loading-overlay')).toHaveClass(/hidden/);
  await page.locator('#toggle-export-menu').click();
  const [download] = await Promise.all([
    page.waitForEvent('download'),
    page.locator('#export-drawio').click(),
  ]);
  expect(download.suggestedFilename()).toBe('architecture-diagram.drawio');
});
