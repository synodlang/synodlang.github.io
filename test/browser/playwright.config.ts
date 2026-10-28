// Browser tests of the built site. Build it first (from the repository root):
//
//   go run ./tools/build fetch && go run ./tools/build build -out _site
//
// then `npm ci && npx playwright install chromium && npx playwright test` here.
// SITE_DIR and SITE_PORT choose another site directory or port.

import { defineConfig, devices } from "@playwright/test";

const port = Number(process.env.SITE_PORT ?? 8123);
// PW_CHROMIUM_CHANNEL=chrome uses an installed Chrome instead of the downloaded Chromium.
const channel = process.env.PW_CHROMIUM_CHANNEL;

export default defineConfig({
  testDir: ".",
  testMatch: "*.spec.ts",
  outputDir: "test-results",
  fullyParallel: true,
  workers: process.env.CI ? 2 : undefined,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  timeout: 180_000,
  expect: { timeout: 60_000 },
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    trace: "retain-on-failure",
    ...devices["Desktop Chrome"],
    viewport: { width: 1280, height: 900 },
    ...(channel ? { channel } : {}),
  },
  webServer: {
    command: "node serve.ts",
    url: `http://127.0.0.1:${port}/`,
    reuseExistingServer: !process.env.CI,
    timeout: 30_000,
  },
});
