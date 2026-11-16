import { test as base } from '@playwright/test'
export { expect } from '@playwright/test'

export const test = base.extend({
  page: async ({ page }, use) => {
    await page.route('**/api/features', route =>
      route.fulfill({ json: { evals: { enabled: false } } })
    )
    await use(page)
  },
})
