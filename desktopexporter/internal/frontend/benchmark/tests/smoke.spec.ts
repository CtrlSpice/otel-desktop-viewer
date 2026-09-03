import { expect, test } from '@playwright/test'

test('serves only the benchmark entrypoint', async ({ page }) => {
  await page.goto('/benchmark/')

  await expect(page).toHaveTitle('Trace Waterfall Benchmark')
  await expect(page.locator('#app')).toHaveAttribute(
    'data-benchmark-sentinel',
    '__WATERFALL_BENCHMARK__'
  )
  await expect(page.locator('#app')).toHaveText(
    'Trace waterfall benchmark Arms A and C ready'
  )
  expect(
    await page.evaluate(
      () => typeof window.__TRACE_WATERFALL_BENCHMARK__?.runArmA
    )
  ).toBe('function')
  expect(
    await page.evaluate(
      () => typeof window.__TRACE_WATERFALL_BENCHMARK__?.runArmC
    )
  ).toBe('function')
})
