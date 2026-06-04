import { defineConfig, devices } from "@playwright/test";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

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

const PORT_API = "48080";
const PORT_WEB = "3000";
const API_URL = `http://localhost:${PORT_API}`;

const isCI = !!process.env.CI;

// Allocate a per-process temp dir for the E2E SQLite file. We
// previously hard-coded `/tmp/opc-e2e.db`, which had two problems:
// (1) two parallel workers on the same host would race on the same
// file and corrupt each other, and (2) the same path was reused
// for any local dev DB the operator had open, so `wipeAll` in
// tests could nuke non-test data. `mktemp` gives us a unique
// directory per process; cleanup is best-effort on SIGINT.
const e2eTmpDir = mkdtempSync(join(tmpdir(), "opc-e2e-"));
const DB_PATH = join(e2eTmpDir, "opc-e2e.db");

export default defineConfig({
  testDir: "./e2e",
  // Mirror the @/lib/* import path the pages use, so spec files can
  // import the API client and helper types without a separate build.
  outputDir: "./e2e/.results",
  // fullyParallel is disabled because every spec's beforeEach wipes
  // the (shared) SQLite DB. Two specs running in parallel can each
  // wipe the table under the other, producing intermittent
  // "no such table" / 4xx-on-create failures that look like real
  // regressions. Each spec opts into parallelism via
  // `test.describe.configure({ mode: 'serial' })` or by being
  // fully isolated; the default is the safer "one spec at a time"
  // mode.
  fullyParallel: false,
  forbidOnly: !!isCI,
  retries: isCI ? 2 : 0,
  workers: isCI ? 2 : 1,
  timeout: 10_000,
  expect: {
    timeout: 5_000,
  },
  reporter: [["list"], ["html", { open: "never" }]],
  use: {
    baseURL: `http://localhost:${PORT_WEB}`,
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  // Spin up both servers in parallel before the first spec runs.
  // Playwright will tear them both down at the end of the run. We
  // re-use an already-running instance (reuseExistingServer: true) so
  // local dev doesn't pay the cold-start cost on every `npx playwright
  // test`.
  webServer: [
    {
      command: `cd ../api && PORT=${PORT_API} DB_PATH=${DB_PATH} GIN_MODE=release ./bin/server`,
      url: `${API_URL}/healthz`,
      reuseExistingServer: true,
      timeout: 30_000,
      stdout: "pipe",
      stderr: "pipe",
    },
    {
      command: `NEXT_PUBLIC_API_URL=${API_URL} npx next start -p ${PORT_WEB}`,
      url: `http://localhost:${PORT_WEB}`,
      reuseExistingServer: true,
      timeout: 60_000,
      stdout: "pipe",
      stderr: "pipe",
    },
  ],
});
