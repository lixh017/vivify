import { defineConfig, devices } from '@playwright/test'

// Playwright configuration for the OPC web app.
//
// - testDir: ./e2e
// - baseURL: http://localhost:3000  (matches the `web` webServer below)
// - webServer: starts the Go API on :48080 and `next start` on :3000 in
//   parallel. We point Next at the API via NEXT_PUBLIC_API_URL so the
//   existing /api/:path* rewrite forwards traffic to the right port.
// - timeouts: 10s per test; 30s for the global setup because both
//   servers need to come up before the first spec runs.
// - retries: 0 in dev, 2 on CI so flakes don't block a green run.
// - reporters: list output to stdout + an HTML report on disk.

const PORT_API = '48080'
const PORT_WEB = '3000'
const API_URL = `http://localhost:${PORT_API}`

const isCI = !!process.env.CI

export default defineConfig({
  testDir: './e2e',
  // Mirror the @/lib/* import path the pages use, so spec files can
  // import the API client and helper types without a separate build.
  outputDir: './e2e/.results',
  fullyParallel: true,
  forbidOnly: !!isCI,
  retries: isCI ? 2 : 0,
  workers: isCI ? 2 : undefined,
  timeout: 10_000,
  expect: {
    timeout: 5_000,
  },
  reporter: [
    ['list'],
    ['html', { open: 'never' }],
  ],
  use: {
    baseURL: `http://localhost:${PORT_WEB}`,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  // Spin up both servers in parallel before the first spec runs.
  // Playwright will tear them both down at the end of the run. We
  // re-use an already-running instance (reuseExistingServer: true) so
  // local dev doesn't pay the cold-start cost on every `npx playwright
  // test`.
  webServer: [
    {
      command: `cd ../api && PORT=${PORT_API} DB_PATH=/tmp/opc-e2e.db GIN_MODE=release ./bin/server`,
      url: `${API_URL}/healthz`,
      reuseExistingServer: true,
      timeout: 30_000,
      stdout: 'pipe',
      stderr: 'pipe',
    },
    {
      command: `NEXT_PUBLIC_API_URL=${API_URL} npx next start -p ${PORT_WEB}`,
      url: `http://localhost:${PORT_WEB}`,
      reuseExistingServer: true,
      timeout: 60_000,
      stdout: 'pipe',
      stderr: 'pipe',
    },
  ],
})
