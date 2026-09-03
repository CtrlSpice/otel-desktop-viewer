import { defineConfig, devices } from '@playwright/test'
import { fileURLToPath, URL } from 'node:url'

const baseURL = 'http://127.0.0.1:4174'
const benchmarkURL = `${baseURL}/benchmark/`
const frontendRoot = fileURLToPath(new URL('..', import.meta.url))
const repositoryRoot = fileURLToPath(new URL('../../../..', import.meta.url))

export default defineConfig({
  testDir: './tests',
  outputDir: '../test-results/benchmark',
  fullyParallel: false,
  forbidOnly: true,
  retries: 0,
  workers: 1,
  reporter: [['list']],
  use: {
    baseURL,
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: [
    {
      command:
        'go run -tags=waterfallbench ./desktopexporter/internal/cmd/waterfallbench serve --listen=127.0.0.1:8001',
      cwd: repositoryRoot,
      url: 'http://127.0.0.1:8001/',
      reuseExistingServer: false,
      timeout: 180_000,
      gracefulShutdown: { signal: 'SIGTERM', timeout: 25_000 },
    },
    {
      command: 'npm run preview:benchmark',
      cwd: frontendRoot,
      url: benchmarkURL,
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
})
