import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page } from '@playwright/test'

async function enableEvals(page: Page, enabled = true) {
  await page.route('**/api/features', route =>
    route.fulfill({
      json: { evals: { enabled, label: 'Experimental and unsupported' } },
    })
  )
}

test('Evals stays closed when its capability is disabled', async ({ page }) => {
  await enableEvals(page, false)
  await page.goto('/evals')
  await expect(
    page.getByRole('heading', { name: 'Evals is not enabled' })
  ).toBeVisible()
  await expect(
    page.getByRole('link', { name: 'Evals', exact: true })
  ).toHaveCount(0)
  await expect(page.getByRole('table')).toHaveCount(0)
})

test('run selection, assessments, filtering and browser history stay connected', async ({
  page,
}) => {
  await enableEvals(page)
  await page.goto('/evals')
  await expect(
    page.getByRole('region', { name: 'Guide-assisted agent', exact: true })
  ).toBeVisible()
  await page.getByRole('button', { name: /Find the slow request/ }).click()
  await page.getByRole('tab', { name: /^Assessments/ }).click()
  await expect(
    page.getByText('The causal claim about total latency', { exact: false })
  ).toBeVisible()
  await page.getByRole('tab', { name: /^Tool calls/ }).click()
  await page.locator('summary').filter({ hasText: 'Error' }).click()
  await expect(
    page.getByText('SQL binder error:', { exact: false })
  ).toBeVisible()
  await page.reload()
  await expect(page.getByRole('tab', { name: /^Tool calls/ })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  await page
    .getByRole('searchbox', { name: 'Filter loaded results' })
    .fill('Mixed')
  await expect(
    page
      .getByRole('table', { name: 'Results for Guide-assisted agent' })
      .getByRole('row')
  ).toHaveCount(2)
  await page
    .getByRole('searchbox', { name: 'Filter loaded results' })
    .fill('nonexistent-score')
  await expect(
    page.getByText('No matching attempts', { exact: true })
  ).toBeVisible()
  await page.getByRole('button', { name: 'Clear filters' }).click()
  await page.getByRole('button', { name: /Direct agent/ }).click()
  await expect(
    page.getByRole('region', { name: 'Direct agent', exact: true })
  ).toBeVisible()
  await page.getByRole('button', { name: /Compare CPU evidence/ }).click()
  await page.getByRole('tab', { name: 'Fields', exact: true }).click()
  await expect(
    page.getByText('The investigator process exited', { exact: false })
  ).toBeVisible()
  await expect(
    page.getByRole('tabpanel').getByText('Unknown', { exact: true })
  ).toHaveCount(2)
  await page.goBack()
  await expect(
    page.getByRole('table', { name: 'Grading for this run' })
  ).toBeVisible()
  await page.getByRole('button', { name: /Revised instructions/ }).click()
  const plannedResults = page.getByRole('table', {
    name: 'Results for Revised instructions',
  })
  await expect(
    plannedResults.getByText('Unscored', { exact: true })
  ).toHaveCount(24)
  await expect(
    page
      .getByRole('table', { name: 'Grading for this run' })
      .getByText('Unknown', { exact: true })
  ).toHaveCount(2)
})

for (const theme of ['rose-pine', 'rose-pine-dawn']) {
  for (const width of [1440, 390]) {
    test(`Evals is accessible in ${theme} at ${width}px`, async ({ page }) => {
      await enableEvals(page)
      await page.setViewportSize({ width, height: 900 })
      await page.addInitScript(
        ({ theme, width }) => {
          localStorage.setItem('theme', theme)
          if (width < 600) localStorage.setItem('signal-drawer:open', 'false')
        },
        { theme, width }
      )
      await page.goto(
        '/evals?run=guide-assisted&attempt=request-latency&detail=Assessments'
      )
      await expect(
        page.getByRole('tabpanel', { name: /^Assessments/ })
      ).toBeVisible()
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth
        )
      ).toBe(true)
      const scan = await new AxeBuilder({ page })
        .withTags(['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa'])
        .analyze()
      expect(scan.violations).toEqual([])
    })
  }
}
