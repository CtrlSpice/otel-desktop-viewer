import { expect, test } from '@playwright/test'

test('serves only the benchmark entrypoint', async ({ page }) => {
  await page.goto('/benchmark/')

  await expect(page).toHaveTitle('Trace Waterfall Benchmark')
  await expect(page.locator('#app')).toHaveAttribute(
    'data-benchmark-sentinel',
    '__WATERFALL_BENCHMARK__'
  )
})
