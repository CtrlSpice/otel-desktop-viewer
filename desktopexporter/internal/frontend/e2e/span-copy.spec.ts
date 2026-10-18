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

for (const [theme, errorColour] of [
  ['rose-pine', 'rgb(235, 111, 146)'],
  ['rose-pine-moon', 'rgb(235, 111, 146)'],
  ['rose-pine-dawn', 'rgb(180, 99, 122)'],
]) {
  test(`span status values use the theme error colour only for code 2 in ${theme}`, async ({
    page,
  }) => {
    let statusCodeValue = 2
    await page.addInitScript(
      theme => localStorage.setItem('theme', theme),
      theme
    )
    await page.setViewportSize({ width: 1920, height: 1100 })
    await page.route('**/rpc', async route => {
      const request = route.request().postDataJSON() as {
        id: number
        method: string
      }
      const statusTrace: JsonTraceData = {
        ...trace,
        spans: trace.spans.map(node =>
          node.spanData.spanID === selectedID
            ? {
                ...node,
                spanData: {
                  ...node.spanData,
                  statusCodeValue,
                  statusCode:
                    statusCodeValue === 2
                      ? 'Error'
                      : statusCodeValue === 0
                        ? 'Unset'
                        : statusCodeValue === 1
                          ? 'Ok'
                          : 'Unknown (99)',
                  statusMessage: 'received error message',
                },
              }
            : node
        ),
      }
      const result =
        request.method === 'getTraceView'
          ? statusTrace
          : request.method === 'searchTraceSummaries'
            ? [
                {
                  traceID,
                  hasRootSpan: true,
                  rootSpan: { serviceName: 'checkout', name: 'other span' },
                  startTime: trace.traceStart,
                  durationNs: '1000000000',
                  spanCount: 2,
                  errorCount: statusCodeValue === 2 ? 1 : 0,
                },
              ]
            : request.method === 'getStats'
              ? {
                  storage: { sizeBytes: 0, maxSizeBytes: 0 },
                  traces: {
                    traceCount: 1,
                    spanCount: 2,
                    serviceCount: 1,
                    errorCount: statusCodeValue === 2 ? 1 : 0,
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
    for (const code of [2, 0, 1, 99]) {
      statusCodeValue = code
      await page.goto(`/traces/${traceID}?span=${selectedID}`)
      const detail = page.getByRole('region', {
        name: 'Span details',
        exact: true,
      })
      const codeField = detail.locator('[data-field="status.code"]')
      const codeValue = codeField.locator('.detail-property__value')
      await expect(codeValue).toBeVisible()
      const message = detail.locator(
        '[data-field="status.message"] .attribute-value'
      )
      const nameValue = detail
        .locator('[data-field="name"] .attribute-value')
        .filter({ hasText: 'selected span' })
      const neutralColour = await nameValue.evaluate(
        value => getComputedStyle(value).color
      )
      await expect(codeValue).toHaveCSS(
        'color',
        code === 2 ? errorColour : neutralColour
      )
      if (code === 2)
        await expect(
          codeField.locator('.detail-property__annotation')
        ).toHaveCSS('color', errorColour)
      if (code === 0 || code === 1) await expect(message).toHaveCount(0)
      else {
        await expect(message).toHaveText('received error message')
        await expect(message).toHaveCSS(
          'color',
          code === 2 ? errorColour : neutralColour
        )
      }
      await expect(nameValue).toHaveCSS('color', neutralColour)
      await expect(codeField.locator('.detail-property__type')).toHaveClass(
        /badge-primary/
      )
    }
  })
}

test('Waterfall columns persist across traces and align through resizing and overflow', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1100 })
  const nextTraceID = 'abcdefabcdefabcdefabcdefabcdefab'
  await page.route('**/rpc', async route => {
    const request = route.request().postDataJSON() as {
      id: number
      method: string
    }
    const nextTrace = page.url().includes(nextTraceID)
    const result =
      request.method === 'getTraceView'
        ? {
            ...trace,
            traceID: nextTrace ? nextTraceID : traceID,
            spans: trace.spans.map(node => ({
              ...node,
              spanData: {
                ...node.spanData,
                attributes: nextTrace
                  ? []
                  : [
                      {
                        key: 'request.count',
                        value: { kind: 'int64', value: '9223372036854775807' },
                      },
                      {
                        key: 'kind',
                        value: { kind: 'string', value: 'attribute kind' },
                      },
                    ],
              },
            })),
          }
        : request.method === 'searchTraceSummaries'
          ? [traceID, nextTraceID].map(id => ({
              traceID: id,
              hasRootSpan: true,
              rootSpan: { serviceName: 'checkout', name: 'other span' },
              startTime: trace.traceStart,
              durationNs: '1000000000',
              spanCount: 2,
              errorCount: 0,
            }))
          : request.method === 'getStats'
            ? {
                storage: { sizeBytes: 0, maxSizeBytes: 0 },
                traces: {
                  traceCount: 2,
                  spanCount: 4,
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
  await expect(
    page.getByRole('columnheader', { name: 'Span name', exact: true })
  ).toBeVisible()
  await expect(
    page.getByRole('columnheader', {
      name: 'resource: service.name',
      exact: true,
    })
  ).toBeVisible()
  await page.getByRole('button', { name: 'Columns', exact: true }).click()
  const picker = page.getByRole('group', { name: 'Waterfall columns' })
  await page.getByRole('searchbox', { name: 'Find columns' }).fill('request')
  await picker
    .getByRole('checkbox', { name: 'span: request.count', exact: true })
    .check()
  await expect(picker).toBeVisible()
  await page.keyboard.press('Escape')
  const header = page.getByRole('columnheader', {
    name: 'span: request.count',
    exact: true,
  })
  const row = page.locator(`.waterfall-row[data-span-id="${selectedID}"]`)
  const cell = row.getByRole('gridcell').nth(1)
  await expect(cell).toHaveText('9223372036854775807')
  await expect(cell.locator('span').first()).toHaveCSS('text-align', 'right')
  await expect(
    row.getByRole('gridcell').first().locator('span').first()
  ).toHaveCSS('text-align', 'left')
  await expect(row).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('treegrid')).toHaveAttribute('aria-colcount', '4')
  const before = (await header.boundingBox())!.width
  await page
    .getByRole('separator', {
      name: 'Resize span: request.count column',
      exact: true,
    })
    .press('ArrowRight')
  await expect
    .poll(async () => (await header.boundingBox())!.width)
    .toBeGreaterThan(before)
  const resized = (await header.boundingBox())!.width
  await page.reload()
  await expect(header).toBeVisible()
  await expect
    .poll(async () => Math.abs((await header.boundingBox())!.width - resized))
    .toBeLessThan(2)
  await page.goto(`/traces/${nextTraceID}?span=${selectedID}`)
  await expect(cell).toHaveText('—')
  await page.getByRole('button', { name: 'Columns', exact: true }).click()
  await expect(
    picker.getByRole('checkbox', { name: 'span: request.count', exact: true })
  ).toBeChecked()
  await picker
    .getByRole('checkbox', { name: 'resource: service.name', exact: true })
    .check()
  await page.keyboard.press('Escape')
  await page.setViewportSize({ width: 1000, height: 900 })
  await expect(row.getByRole('gridcell').first()).toHaveText('checkout')
  await expect
    .poll(async () => {
      const h = await header.boundingBox()
      const c = await cell.boundingBox()
      if (!h || !c) return Infinity
      return Math.max(Math.abs(h.x - c.x), Math.abs(h.width - c.width))
    })
    .toBeLessThan(2)
  await page.locator('.waterfall-view__scroll').evaluate(el => {
    el.scrollLeft = el.scrollWidth
  })
  await expect
    .poll(async () =>
      Math.abs((await header.boundingBox())!.x - (await cell.boundingBox())!.x)
    )
    .toBeLessThan(2)
  await page.getByRole('button', { name: 'Columns', exact: true }).click()
  await picker
    .getByRole('checkbox', { name: 'span: request.count', exact: true })
    .click()
  await expect(header).toHaveCount(0)
  await expect(row).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('treegrid')).toHaveAttribute('aria-colcount', '3')
  await picker
    .getByRole('checkbox', { name: 'resource: service.name', exact: true })
    .uncheck()
  await expect(page.getByRole('treegrid')).toHaveAttribute('aria-colcount', '2')
  await expect(page.getByRole('columnheader')).toHaveCount(2)
  await page.reload()
  await expect(page.getByRole('treegrid')).toHaveAttribute('aria-colcount', '2')
  await expect(
    page.getByRole('columnheader', { name: 'Span name', exact: true })
  ).toBeVisible()
  await expect(
    page.getByRole('columnheader', {
      name: 'resource: service.name',
      exact: true,
    })
  ).toHaveCount(0)
  await page.goto(`/traces/${traceID}?span=${selectedID}`)
  await page.getByRole('button', { name: 'Columns', exact: true }).click()
  await page.getByRole('searchbox', { name: 'Find columns' }).fill('kind')
  await picker
    .getByRole('checkbox', { name: 'span field: kind', exact: true })
    .check()
  await picker
    .getByRole('checkbox', { name: 'span: kind', exact: true })
    .check()
  await page.keyboard.press('Escape')
  await expect(row.getByRole('gridcell').nth(0)).toHaveText('2 (Server)')
  await expect(row.getByRole('gridcell').nth(1)).toHaveText('attribute kind')
  await page.reload()
  await expect(
    page.getByRole('columnheader', { name: 'span field: kind', exact: true })
  ).toBeVisible()
  await expect(
    page.getByRole('columnheader', { name: 'span: kind', exact: true })
  ).toBeVisible()
  await expect(row.getByRole('gridcell').nth(0)).toHaveText('2 (Server)')
  await expect(row.getByRole('gridcell').nth(1)).toHaveText('attribute kind')
})

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
