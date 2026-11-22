import { test, expect } from '@playwright/test';
import { spawn } from 'node:child_process';

test('offline summary endpoint returns a deterministic brief', async ({ request }) => {
  const res = await request.get('/api/summary');
  expect(res.ok()).toBeTruthy();
  const body = await res.json();
  expect(body.source).toBe('local');
  expect(body.summary).toContain('Architecture summary');
  expect(body.summary).toContain('resources');
});

test('cost overlay returns totals and an honest disclaimer', async ({ request }) => {
  const res = await request.get('/api/costs');
  expect(res.ok()).toBeTruthy();
  const body = await res.json();
  expect(body.currency).toBe('USD');
  expect(body.total_monthly_usd).toBeGreaterThan(0);
  expect(body.disclaimer).toContain('Not a bill');
  expect(Array.isArray(body.resources)).toBeTruthy();
});

test('draw.io export is served from the embedded exporter', async ({ request }) => {
  const res = await request.get('/api/export/drawio');
  expect(res.ok()).toBeTruthy();
  expect(res.headers()['content-type']).toContain('application/xml');
  const text = await res.text();
  expect(text).toContain('<mxfile');
  expect(text).toContain('<mxGraphModel');
});

test('local snapshot lifecycle: save, list, restore, delete', async ({ request }) => {
  const saved = await request.post('/api/snapshots', { data: { name: 'e2e-snapshot' } });
  expect(saved.ok()).toBeTruthy();
  const snap = await saved.json();
  expect(snap.name).toBe('e2e-snapshot');
  expect(snap.items).toBe(1);

  const list = await request.get('/api/snapshots');
  expect((await list.json()).some((s) => s.id === snap.id)).toBeTruthy();

  const restore = await request.get(`/api/snapshots/${snap.id}`);
  expect(restore.ok()).toBeTruthy();
  expect((await restore.json()).success).toBeTruthy();

  const del = await request.delete(`/api/snapshots/${snap.id}`);
  expect(del.ok()).toBeTruthy();
  const after = await request.get('/api/snapshots');
  expect((await after.json()).some((s) => s.id === snap.id)).toBeFalsy();
});

test('share bundle is portable across machines', async ({ request }) => {
  const bundle = await (await request.get('/api/bundle')).json();
  expect(bundle.version).toBe(1);
  expect(bundle.states).toHaveLength(1);
  expect(bundle.states[0].content).toContain('"version":4');

  const res = await request.post('/api/bundle', { data: bundle });
  expect(res.ok()).toBeTruthy();
  expect((await res.json()).states).toBe(1);
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

test('insights drawer renders summary, costs and a share URL', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('#statStates')).toHaveText('1', { timeout: 15_000 });

  await page.locator('#insightsBtn').click();
  await expect(page.locator('#insightsOverlay')).toBeVisible();

  await page.locator('#genSummaryBtn').click();
  await expect(page.locator('#summaryOut')).toContainText('Architecture summary');

  await page.locator('#genCostsBtn').click();
  await expect(page.locator('#costsOut')).toContainText('Estimated total:');

  // On localhost a URL cannot work for another computer: the UI must say so.
  await page.locator('#copyShareBtn').click();
  await expect(page.locator('#shareNote')).toContainText('loopback address');
  await expect(page.locator('#shareUrl')).toHaveValue('');

  const [bundleDownload] = await Promise.all([
    page.waitForEvent('download'),
    page.locator('#exportBundleBtn').click(),
  ]);
  expect(bundleDownload.suggestedFilename()).toBe('terraviz-share.json');

  await page.locator('#closeInsightsBtn').click();
  await expect(page.locator('#insightsOverlay')).toBeHidden();
});

test('draw.io export button downloads an editable file', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('#statStates')).toHaveText('1', { timeout: 15_000 });
  await page.locator('#exportMenuBtn').click();
  const [download] = await Promise.all([
    page.waitForEvent('download'),
    page.locator('#drawioExportBtn').click(),
  ]);
  expect(download.suggestedFilename()).toBe('architecture-diagram.drawio');
});

// Keep this last: it loads all bundled samples, which changes the shared DB.
test('architecture drill-down shows visible dependency arrows', async ({ page, request }) => {
  await request.post('/api/samples/ingest', { data: { samples: [] } });
  await page.goto('/#state=acme-network');
  await expect(page.locator('#graph g.node').first()).toBeVisible({ timeout: 15_000 });
  const arrows = page.locator('#graph line.dep-line');
  expect(await arrows.count()).toBeGreaterThan(0);
  await expect(arrows.first()).toHaveAttribute('marker-end', 'url(#arrow-dep)');
  await expect(page.locator('#graph marker#arrow-dep')).toHaveCount(1);
});
