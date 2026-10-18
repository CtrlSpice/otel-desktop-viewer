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
  const toolbar = page.getByRole('toolbar', {
    name: 'Workspace controls',
  })
  await expect(
    toolbar.getByRole('searchbox', { name: 'Search evals' })
  ).toBeVisible()
  await expect(
    toolbar.getByRole('searchbox', { name: 'Search evals' })
  ).toHaveCSS('font-size', '14px')
  expect(
    await toolbar.getByRole('search').evaluate(element =>
      [...element.querySelectorAll('button')].flatMap(button => {
        if (button.closest('[popover], dialog')) return []
        const name = button.getAttribute('aria-label')
        return [name?.startsWith('Change time range') ? 'Time' : name]
      })
    )
  ).toEqual(['Refresh', 'Clear search', 'Search query help', 'Time'])
  await expect
    .poll(() =>
      toolbar.getByRole('search').evaluate(element => {
        const surface = element.querySelector('.search-bar__query')!
        const help = element.querySelector(
          'button[aria-label="Search query help"]'
        )!
        return Math.abs(
          surface.getBoundingClientRect().right -
            help.getBoundingClientRect().right
        )
      })
    )
    .toBeLessThan(1)
  await expect(
    toolbar.getByRole('button', { name: 'Refresh', exact: true })
  ).toBeVisible()
  expect(
    await toolbar
      .getByRole('button', { name: 'Refresh', exact: true })
      .evaluate(element => getComputedStyle(element).backgroundColor)
  ).toBe('rgba(0, 0, 0, 0)')
  await expect(page.getByRole('searchbox')).toHaveCount(1)
  await expect(
    page.getByRole('combobox', { name: 'Execution status' })
  ).toHaveCount(0)
  await expect(
    page.getByRole('combobox', { name: 'Sort results' })
  ).toHaveCount(0)
  await expect(
    toolbar.getByRole('button', { name: /^Change time range/ })
  ).toHaveText('')
  await expect(
    page.getByRole('region', { name: 'Guide-assisted agent', exact: true })
  ).toBeVisible()
  await page.getByRole('button', { name: /Find the slow request/ }).click()
  const detail = page.getByRole('region', {
    name: 'Attempt details',
    exact: true,
  })
  const bubbles = detail.locator('details.collapse')
  await expect(bubbles).toHaveCount(3)
  await expect(detail.getByRole('tablist')).toHaveCount(0)
  for (const bubble of await bubbles.all()) {
    await expect(bubble).toHaveAttribute('open')
    await expect(bubble.locator(':scope > summary')).toBeInViewport({
      ratio: 1,
    })
  }
  await expect(
    page.getByText('The causal claim about total latency', { exact: false })
  ).toBeVisible()
  await page.locator('summary').filter({ hasText: 'Error' }).click()
  await expect(
    page.getByText('SQL binder error:', { exact: false })
  ).toBeVisible()
  await page.reload()
  await expect(bubbles.nth(2).locator(':scope > summary')).toHaveText(
    /Tool calls/
  )
  await expect(bubbles.nth(2)).toHaveAttribute('open')
  await page.getByRole('searchbox', { name: 'Search evals' }).fill('Mixed')
  await expect(
    page
      .getByRole('table', { name: 'Results for Guide-assisted agent' })
      .getByRole('row')
  ).toHaveCount(2)
  await page
    .getByRole('searchbox', { name: 'Search evals' })
    .fill('nonexistent-score')
  await expect(
    page.getByText('No matching attempts', { exact: true })
  ).toBeVisible()
  await toolbar.getByRole('button', { name: 'Clear search' }).click()
  await page.getByRole('button', { name: /Direct agent/ }).click()
  await expect(
    page.getByRole('region', { name: 'Direct agent', exact: true })
  ).toBeVisible()
  await page.getByRole('button', { name: /Compare CPU evidence/ }).click()
  await expect(
    page.getByText('The investigator process exited', { exact: false })
  ).toBeVisible()
  await expect(
    bubbles.nth(0).getByText('Unknown', { exact: true })
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

test('detail actions reopen the panel, including the selected attempt', async ({
  page,
}) => {
  await enableEvals(page)
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.addInitScript(() =>
    localStorage.setItem('signal-detail:open', 'false')
  )
  await page.goto('/evals')
  const detail = page.locator('#signal-drawer-detail')
  await expect(detail).toBeHidden()
  const attempt = page.getByRole('button', { name: /Find the slow request/ })
  for (let selection = 0; selection < 2; selection++) {
    await attempt.click()
    await expect(detail).toBeVisible()
    await expect(page).toHaveURL(/attempt=request-latency/)
    await expect(
      detail.getByRole('region', { name: 'Attempt details', exact: true })
    ).toBeVisible()
    const fields = detail.locator('details.collapse').first()
    await expect(fields).toHaveAttribute('open')
    await fields.locator(':scope > summary').click()
    await expect(fields).not.toHaveAttribute('open')
    await expect(detail.locator('details.collapse').nth(1)).toHaveAttribute(
      'open'
    )
    await fields.locator(':scope > summary').click()
    await page.getByRole('button', { name: 'Collapse details' }).click()
    await expect(detail).toBeHidden()
  }
  await page.getByRole('button', { name: 'Run details', exact: true }).click()
  await expect(
    detail.getByRole('region', { name: 'Run details', exact: true })
  ).toBeVisible()
  expect(new URL(page.url()).searchParams.has('attempt')).toBe(false)
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
      await page.goto('/evals?run=guide-assisted&attempt=request-latency')
      await expect(
        page.getByRole('region', { name: 'Attempt details', exact: true })
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
