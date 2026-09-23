import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [["list"], ["html", { open: "never" }]],
  use: {
    baseURL: "http://127.0.0.1:3000",
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
    video: "off",
    ...devices["Desktop Chrome"],
  },
  webServer: [
    {
      command: "bash ../e2e/start-gateway.sh",
      url: "http://127.0.0.1:4000/health/liveliness",
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: "npx next start -p 3000",
      url: "http://127.0.0.1:3000/login",
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
});
