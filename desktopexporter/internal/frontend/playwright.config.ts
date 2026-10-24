import { defineConfig, devices } from '@playwright/test'

const evals = process.env.EVALS ?? '0'
if (evals !== '0' && evals !== '1') throw new Error('EVALS must be 0 or 1')
const port = evals === '1' ? 3003 : 3002
const baseURL = `http://127.0.0.1:${port}`

export default defineConfig({
  testDir: './e2e',
  testIgnore:
    evals === '1'
      ? []
      : [
          '**/evals.spec.ts',
          '**/eval-source-copy.spec.ts',
          '**/eval-mounted-state.spec.ts',
        ],
  outputDir: 'test-results',
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: [
    ['list'],
    ['html', { open: 'never', outputFolder: 'playwright-report' }],
  ],
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
  webServer: {
    command: `npm run dev -- --host 127.0.0.1 --port ${port}`,
    env: { VIEWER_VITE_CACHE_DIR: `node_modules/.vite-playwright-${evals}` },
    url: baseURL,
    reuseExistingServer: false,
    timeout: 120_000,
  },
})
