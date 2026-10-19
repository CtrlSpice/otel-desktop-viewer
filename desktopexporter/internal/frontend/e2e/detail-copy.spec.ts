import { expect, test } from '@playwright/test'

for (const fallback of [false, true]) {
  test(`detail values copy to the clipboard ${fallback ? 'when the async API is denied' : 'through the async API'}`, async ({
    page,
    context,
  }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await page.route('**/api/features', route =>
      route.fulfill({
        json: { evals: { enabled: true } },
      })
    )
    await page.setViewportSize({ width: 1440, height: 1000 })
    await page.goto('/evals?run=guide-assisted&attempt=request-latency')
    const field = page
      .getByRole('table', { name: 'Attempt fields' })
      .locator('[data-field="name"]')
    const copy = field.getByRole('button', { name: 'Copy name', exact: true })
    const expected = await field.locator('.detail-pair__value').innerText()
    await expect(copy).toHaveCSS('opacity', '0')
    const before = (await field.boundingBox())!
    await copy.focus()
    await expect(copy).toHaveCSS('opacity', '1')
    await copy.evaluate(element => (element as HTMLElement).blur())
    await expect(copy).toHaveCSS('opacity', '0')
    await field.hover()
    await expect(copy).toHaveCSS('opacity', '1')
    expect((await field.boundingBox())!.height).toBe(before.height)
    await page.evaluate(() =>
      navigator.clipboard.writeText('clipboard unchanged')
    )
    await field.locator('.detail-property__body').click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
      'clipboard unchanged'
    )
    if (fallback) {
      await page.evaluate(() => {
        Object.defineProperty(navigator.clipboard, 'writeText', {
          configurable: true,
          value: () =>
            Promise.reject(new DOMException('Denied', 'NotAllowedError')),
        })
      })
    }
    if (fallback) {
      await copy.focus()
      await page.keyboard.press('Enter')
    } else await copy.click()
    await expect(field.getByRole('status')).toHaveText('Copied name.')
    await expect(copy).toHaveCSS('opacity', '1')
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(expected)
    if (fallback) await expect(copy).toBeFocused()
    await expect(page.locator('textarea[aria-hidden="true"]')).toHaveCount(0)
  })
}

test('touch devices keep Copy visible and copy only through its button', async ({
  browser,
  baseURL,
}) => {
  const context = await browser.newContext({
    baseURL,
    viewport: { width: 390, height: 900 },
    hasTouch: true,
    isMobile: true,
    permissions: ['clipboard-read', 'clipboard-write'],
  })
  try {
    const page = await context.newPage()
    await page.addInitScript(() =>
      localStorage.setItem('signal-drawer:open', 'false')
    )
    await page.route('**/api/features', route =>
      route.fulfill({ json: { evals: { enabled: true } } })
    )
    await page.goto('/evals?run=guide-assisted&attempt=request-latency')
    const field = page
      .getByRole('table', { name: 'Attempt fields' })
      .locator('[data-field="name"]')
    const copy = field.getByRole('button', { name: 'Copy name', exact: true })
    await expect(copy).toHaveCSS('opacity', '1')
    const expected = await field.locator('.detail-pair__value').innerText()
    await page.evaluate(() =>
      navigator.clipboard.writeText('clipboard unchanged')
    )
    await field.locator('.detail-property__body').tap()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
      'clipboard unchanged'
    )
    await copy.tap()
    await expect(copy).toHaveCSS('opacity', '1')
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(expected)
  } finally {
    await context.close()
  }
})
