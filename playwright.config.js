// Playwright configuration. The web server is the real TerraViz binary serving
// the checked-in diagram fixture, so tests exercise the embedded UI and API.
import fs from 'node:fs';
import { defineConfig, devices } from '@playwright/test';

const PORT = 4173;
const HOST = '127.0.0.1';
const HOME = '/tmp/opencode/terraviz-e2e-home';

// Keep snapshot tests isolated from the developer's real config directory.
fs.rmSync(HOME, { recursive: true, force: true });
fs.mkdirSync(HOME, { recursive: true });

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: 0,
  reporter: [['list']],
  timeout: 30_000,
  expect: { timeout: 7_000 },
  use: {
    baseURL: `http://${HOST}:${PORT}`,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    ...devices['Desktop Chrome'],
    // Use the Chromium already present in the environment; avoids a download.
    launchOptions: {
      executablePath: process.env.PLAYWRIGHT_CHROMIUM ||
        '/home/krishna/.cache/ms-playwright/chromium-1194/chrome-linux/chrome',
    },
  },
  webServer: {
    command: `./terraviz serve diagram_small.json --addr ${HOST}:${PORT} --db ${HOME}/test.db`,
    env: { ...process.env, TERRAVIZ_HOME: HOME },
    url: `http://${HOST}:${PORT}/terraviz/api/diagram`,
    reuseExistingServer: !process.env.CI,
    timeout: 60_000,
    stdout: 'ignore',
    stderr: 'pipe',
  },
});
