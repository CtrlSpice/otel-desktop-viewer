import { routeTelemetry } from './telemetry-fixtures'
import { expect, test, type Page } from '@playwright/test'
import type { JsonLogData } from '../src/types/wire-types'

// Exercise the shared field card with retained telemetry, independently of
// Evals' run/attempt navigation and its retired presentation-only examples.
async function openCopyField(
  page: Page,
  body = 'Exact retained clipboard text'
) {
  const log: JsonLogData = {
    logRef: '00000000-0000-0000-0000-000000000001',
    timestamp: '1700000000000000000',
    observedTimestamp: '1700000000000000000',
    traceID: null,
    spanID: null,
    severityText: 'INFO',
    severityNumber: 9,
    body: { kind: 'string', value: body },
    resource: { attributes: [], droppedAttributesCount: 0 },
    scope: { name: '', version: '', attributes: [], droppedAttributesCount: 0 },
    attributes: [],
    droppedAttributesCount: 0,
    flags: 0,
    eventName: '',
  }
  await page.addInitScript(() => {
    localStorage.setItem('signal-detail:open', 'true')
    localStorage.setItem('signal-drawer:open', 'false')
  })
  await page.route('**/api/features', route => route.fulfill({ json: {} }))
  await routeTelemetry(page, route => {
    const { id, method } = route.request().postDataJSON()
    const result =
      method === 'getLog'
        ? log
        : method === 'searchLogPage'
          ? { total: 0, logs: [] }
          : method === 'getLogErrorNavigation'
            ? { count: 0, index: null, previous: null, next: null }
            : method === 'discoverTelemetry'
              ? { rows: [], truncated: false }
              : []
    return route.fulfill({ json: { jsonrpc: '2.0', id, result } })
  })
  await page.goto(`/logs/${log.logRef}?time=all`)
  return page
    .getByRole('table', { name: 'Log fields' })
    .locator('[data-field="body"]')
}

for (const structured of [false, true]) {
  test(`long ${structured ? 'JSON detail fields fold' : 'text detail fields use native two-line previews'} and copy the full value`, async ({
    page,
    context,
  }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await page.setViewportSize({ width: 1440, height: 1000 })
    const lines = Array.from({ length: 12 }, (_, i) => `Retained line ${i + 1}`)
    const text = structured ? JSON.stringify({ lines }) : lines.join('\n')
    const field = await openCopyField(page, text)
    if (structured) {
      const editor = field.locator('.cm-editor')
      await expect(editor).toBeVisible()
      await expect(
        field.getByRole('button', { name: 'Show more', exact: true })
      ).toHaveCount(0)
      await field
        .getByRole('button', { name: 'Expand all body JSON', exact: true })
        .click()
      await expect(editor).toContainText('Retained line 12')
      await field
        .getByRole('button', { name: 'Collapse all body JSON', exact: true })
        .click()
      await expect(editor).not.toContainText('Retained line 12')
      await field
        .getByRole('button', { name: 'Copy body', exact: true })
        .click()
      await expect
        .poll(() => page.evaluate(() => navigator.clipboard.readText()))
        .toBe(text)
      return
    }
    const content = field.locator('.detail-pair__value')
    const more = field.getByRole('button', { name: 'Show more', exact: true })
    await expect(more).toBeVisible()
    await expect(content).toHaveCSS('-webkit-line-clamp', '2')
    expect((await content.boundingBox())!.height).toBe(42)
    const previewBox = (await content.boundingBox())!
    expect((await more.boundingBox())!.y).toBeGreaterThanOrEqual(
      previewBox.y + previewBox.height
    )
    await field.hover()
    await field.getByRole('button', { name: 'Copy body', exact: true }).click()
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(text)
    await more.focus()
    await page.keyboard.press('Enter')
    const less = field.getByRole('button', { name: 'Show less', exact: true })
    await expect(less).toBeFocused()
    await expect(less).toHaveAttribute('aria-expanded', 'true')
    expect((await content.boundingBox())!.height).toBeGreaterThan(63)
    await expect(content).toContainText('Retained line 12')
    await less.click()
    await expect(more).toBeVisible()
    expect((await content.boundingBox())!.height).toBe(42)
  })
}

test('a complete three-line detail value needs no disclosure', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 1000 })
  const field = await openCopyField(page, 'First line\nSecond line\nThird line')
  await expect(field.locator('.detail-pair__value')).toBeVisible()
  await expect(
    field.getByRole('button', { name: 'Show more', exact: true })
  ).toHaveCount(0)
  expect(
    (await field.locator('.detail-pair__value').boundingBox())!.height
  ).toBe(63)
})

for (const fallback of [false, true]) {
  test(`detail values copy to the clipboard ${fallback ? 'when the async API is denied' : 'through the async API'}`, async ({
    page,
    context,
  }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await page.setViewportSize({ width: 1440, height: 1000 })
    const field = await openCopyField(page)
    const copy = field.getByRole('button', { name: 'Copy body', exact: true })
    const expected = 'Exact retained clipboard text'
    await expect(field.locator('.detail-property__body')).toHaveText(expected)
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
    await expect(field.getByRole('status')).toHaveText('Copied body.')
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
    const field = await openCopyField(page)
    const copy = field.getByRole('button', { name: 'Copy body', exact: true })
    await expect(copy).toHaveCSS('opacity', '1')
    const expected = 'Exact retained clipboard text'
    await expect(field.locator('.detail-property__body')).toHaveText(expected)
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
