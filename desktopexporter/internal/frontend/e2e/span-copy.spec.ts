import AxeBuilder from '@axe-core/playwright'
import { expect, test } from '@playwright/test'
import type { JsonTraceData } from '../src/types/wire-types'

const traceID = '12345678901234567890123456789012'
const rootID = '1111111111111111'
const selectedID = '2222222222222222'
const trace: JsonTraceData = {
  traceID,
  traceStart: '1700000000123456789',
  resources: {
    0: {
      attributes: [
        { key: 'service.name', value: { kind: 'string', value: 'checkout' } },
      ],
      droppedAttributesCount: 0,
    },
  },
  scopes: {
    0: {
      name: 'instrumentation',
      version: '1.2',
      attributes: [],
      droppedAttributesCount: 0,
    },
  },
  unplacedSpanCount: 0,
  spans: [rootID, selectedID].map((spanID, index) => ({
    depth: index,
    matched: true,
    spanData: {
      spanID,
      parentSpanID: index === 0 ? null : rootID,
      name: index === 0 ? 'other span' : 'selected span',
      traceState: '',
      flags: 1,
      kindCode: 2,
      kind: 'Server',
      start: '0',
      dur: '1000000000',
      r: 0,
      s: 0,
      attributes: [
        {
          key: 'sequence',
          value: { kind: 'int64', value: '9223372036854775807' },
        },
        {
          key: 'payload',
          value: {
            kind: 'map',
            value: [
              { key: 'name', value: { kind: 'string', value: 'example' } },
              { key: 'count', value: { kind: 'int64', value: '42' } },
              { key: 'ready', value: { kind: 'bool', value: false } },
            ],
          },
        },
      ],
      links: [],
      events: [
        {
          name: 'event',
          timestamp: '1700000000999999999',
          attributes: [],
          droppedAttributesCount: 0,
        },
      ],
      droppedAttributesCount: 0,
      droppedEventsCount: 0,
      droppedLinksCount: 0,
      statusCodeValue: 1,
      statusCode: 'Ok',
      statusMessage: '',
    },
  })),
}

test('Copy span sits opposite the span icon and copies only the selected span with context', async ({
  page,
  context,
}) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.setViewportSize({ width: 1920, height: 1100 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.addInitScript(() =>
    localStorage.setItem('theme', 'rose-pine-moon')
  )
  await page.route('**/rpc', async route => {
    const request = route.request().postDataJSON() as {
      id: number
      method: string
    }
    const result =
      request.method === 'getTraceView'
        ? trace
        : request.method === 'searchTraceSummaries'
          ? [
              {
                traceID,
                hasRootSpan: true,
                rootSpan: { serviceName: 'checkout', name: 'other span' },
                startTime: trace.traceStart,
                durationNs: '1000000000',
                spanCount: 2,
                errorCount: 0,
              },
            ]
          : request.method === 'getStats'
            ? {
                storage: { sizeBytes: 0, maxSizeBytes: 0 },
                traces: {
                  traceCount: 1,
                  spanCount: 2,
                  serviceCount: 1,
                  errorCount: 0,
                  lastReceived: null,
                },
                logs: { logCount: 0, errorCount: 0, lastReceived: null },
                metrics: {
                  metricCount: 0,
                  dataPointCount: 0,
                  lastReceived: null,
                },
                rejections: [],
              }
            : []
    await route.fulfill({ json: { jsonrpc: '2.0', id: request.id, result } })
  })
  await page.goto(`/traces/${traceID}?span=${selectedID}`)
  const hoveredRow = page.locator(`.waterfall-row[data-span-id="${rootID}"]`)
  await hoveredRow.hover()
  await expect(hoveredRow).toHaveCSS('border-radius', '12px')
  await expect(hoveredRow).toHaveAttribute('aria-selected', 'false')
  const detail = page.getByRole('region', { name: 'Span details', exact: true })
  const bubble = detail.locator('.detail-bubble__with-action')
  const summary = bubble.locator('summary').first()
  const copy = detail.getByRole('button', { name: 'Copy span', exact: true })
  await expect(copy).toBeVisible()
  await expect(copy).toHaveCSS('opacity', '1')
  const iconBox = (await summary.locator('svg').boundingBox())!
  const copyBox = (await copy.boundingBox())!
  const headerBox = (await summary.boundingBox())!
  expect(
    Math.abs(iconBox.y + iconBox.height / 2 - copyBox.y - copyBox.height / 2)
  ).toBeLessThan(1)
  expect(headerBox.x + headerBox.width - copyBox.x - copyBox.width).toBeCloseTo(
    12,
    0
  )
  const valueBox = (await detail
    .getByRole('table', { name: 'Span fields' })
    .locator('.detail-property__body')
    .first()
    .boundingBox())!
  expect(valueBox.x).toBeCloseTo(iconBox.x, 0)
  const palette = await page.evaluate(() => {
    const probe = document.createElement('span')
    document.body.append(probe)
    const colors = Object.fromEntries(
      ['base-content', 'accent', 'warning', 'primary'].map(token => {
        probe.style.color = `var(--color-${token})`
        return [token, getComputedStyle(probe).color]
      })
    )
    probe.remove()
    return colors
  })
  const fields = detail.getByRole('table', { name: 'Span fields' })
  await expect(
    fields.locator('[data-field="name"]').first().locator('.detail-pair__value')
  ).toHaveCSS('color', palette['base-content']!)
  await expect(
    fields
      .locator('[data-field="name"]')
      .first()
      .locator('.detail-property__type')
  ).toHaveCSS('color', palette.accent!)
  await expect(
    fields.locator('[data-field="kind"] .detail-property__value')
  ).toHaveCSS('color', palette['base-content']!)
  await expect(
    fields.locator(
      '[data-field="startTimeUnixNano"] .detail-property__raw-timestamp'
    )
  ).toHaveCSS('color', palette['base-content']!)
  await expect(
    fields.locator('[data-field="sequence"] .detail-code--number')
  ).toHaveCSS('color', palette['base-content']!)
  const json = fields.locator('[data-field="payload"]')
  await expect(json.locator('.detail-code--string')).toHaveCSS(
    'color',
    palette.accent!
  )
  await expect(json.locator('.detail-code--number')).toHaveCSS(
    'color',
    palette.warning!
  )
  await expect(json.locator('.detail-code--literal')).toHaveCSS(
    'color',
    palette.primary!
  )
  expect(await copy.evaluate(element => element.closest('summary'))).toBeNull()
  await copy.click()
  await expect(
    bubble.locator('.detail-bubble__action').getByRole('status')
  ).toHaveText('Copied span.')
  const copied = JSON.parse(
    await page.evaluate(() => navigator.clipboard.readText())
  )
  expect(copied.resourceSpans).toHaveLength(1)
  const resource = copied.resourceSpans[0]
  expect(resource.resource.attributes).toEqual([
    { key: 'service.name', value: { stringValue: 'checkout' } },
  ])
  expect(resource.scopeSpans[0].scope.name).toBe('instrumentation')
  expect(resource.scopeSpans[0].spans).toHaveLength(1)
  expect(resource.scopeSpans[0].spans[0]).toMatchObject({
    name: 'selected span',
    spanId: selectedID,
    parentSpanId: rootID,
    startTimeUnixNano: '1700000000123456789',
    attributes: expect.arrayContaining([
      { key: 'sequence', value: { intValue: '9223372036854775807' } },
      {
        key: 'payload',
        value: {
          kvlistValue: {
            values: [
              { key: 'name', value: { stringValue: 'example' } },
              { key: 'count', value: { intValue: '42' } },
              { key: 'ready', value: { boolValue: false } },
            ],
          },
        },
      },
    ]),
  })
  await expect(bubble.locator('details').first()).toHaveAttribute('open')
  await expect(page).toHaveURL(new RegExp(`span=${selectedID}`))
  await summary.click()
  await expect(bubble.locator('details').first()).not.toHaveAttribute('open')
  await expect(copy).toBeVisible()
  const scan = await new AxeBuilder({ page })
    .include('.detail-bubble__with-action')
    .withRules(['nested-interactive'])
    .analyze()
  expect(scan.violations).toEqual([])

  // A real trace search uses Highlight High, not the header's Overlay surface.
  const editor = page.locator('.search-bar .cm-content')
  await editor.fill('name = "selected span"')
  await editor.press('Escape')
  await editor.press('Enter')
  await expect(page.locator('.waterfall-row--matched')).toHaveCount(2)
  await page.locator('.pane-header--pill').hover()
  const match = page.locator('.waterfall-row--matched[aria-selected="false"]')
  await expect(match.locator('td').first()).toHaveCSS(
    'background-color',
    'rgb(86, 82, 110)'
  )
  const selected = page.locator('.waterfall-row[aria-selected="true"]')
  await expect(selected.locator('td').first()).toHaveCSS(
    'background-color',
    'rgb(68, 65, 90)'
  )
  await selected.hover()
  await expect(selected.locator('td').first()).toHaveCSS(
    'background-color',
    'rgb(68, 65, 90)'
  )
})
