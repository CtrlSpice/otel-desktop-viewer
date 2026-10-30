import { routeTelemetry } from './telemetry-fixtures'
import { expect, test, type Page } from '@playwright/test'
import type {
  AttributeFixture,
  LogFixture,
} from '../src/test/telemetry-fixture-types'

// Exercise the shared field card with retained telemetry, independently of
// Evals' run/attempt navigation and its retired presentation-only examples.
async function openCopyField(
  page: Page,
  body: string | AttributeFixture = 'Exact retained clipboard text'
) {
  const log: LogFixture = {
    logRef: '00000000-0000-0000-0000-000000000001',
    timestamp: '1700000000000000000',
    observedTimestamp: '1700000000000000000',
    traceID: null,
    spanID: null,
    severityText: 'INFO',
    severityNumber: 9,
    body: typeof body === 'string' ? { kind: 'string', value: body } : body,
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

test('native Details expands bounded children and finds and copies values beyond the visible batch', async ({
  page,
  context,
}) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  const field = await openCopyField(page, {
    kind: 'map',
    value: [
      {
        key: 'items',
        value: {
          kind: 'array',
          value: Array.from({ length: 250 }, (_, i) => ({
            kind: 'map',
            value: [
              { key: 'index', value: { kind: 'int64', value: String(i) } },
              {
                key: 'exact',
                value: { kind: 'int64', value: '9223372036854775807' },
              },
              {
                key: 'text',
                value: {
                  kind: 'string',
                  value: i === 249 ? 'last-hidden-value' : 'ordinary',
                },
              },
            ],
          })),
        },
      },
    ],
  })
  await expect(field.locator('.cm-editor')).toHaveCount(0)
  await field.getByRole('button', { name: 'Expand items', exact: true }).click()
  await expect(
    field.getByRole('button', { name: 'Expand 99', exact: true })
  ).toBeVisible()
  await expect(
    field.getByRole('button', { name: 'Expand 100', exact: true })
  ).toHaveCount(0)
  await field.getByRole('button', { name: 'Show next 100 of 250' }).click()
  await expect(
    field.getByRole('button', { name: 'Expand 199', exact: true })
  ).toBeVisible()
  await field
    .getByRole('button', { name: 'Copy body', exact: true })
    .press('Enter')
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toContain('last-hidden-value')
  const exact = await page.evaluate(() => navigator.clipboard.readText())
  expect(exact).toContain('9223372036854775807')
  await field.getByRole('button', { name: 'Full document / Find' }).click()
  const search = field.locator('.cm-search input[name="search"]')
  await expect(search).toBeFocused()
  await search.pressSequentially('last-hidden-value')
  await search.press('Enter')
  await expect(field.locator('.cm-searchMatch-selected')).toHaveText(
    'last-hidden-value'
  )
  await field.getByRole('button', { name: 'Properties', exact: true }).click()
  await expect(
    field.getByRole('button', { name: 'Collapse items', exact: true })
  ).toHaveAttribute('aria-expanded', 'true')
  await expect(
    field.getByRole('button', { name: 'Expand 199', exact: true })
  ).toBeVisible()
  await field.getByRole('button', { name: 'Full document / Find' }).click()
  await field.getByRole('button', { name: 'Collapse all body JSON' }).click()
  await field
    .getByRole('button', { name: 'Copy body', exact: true })
    .press('Enter')
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(exact)
})

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
