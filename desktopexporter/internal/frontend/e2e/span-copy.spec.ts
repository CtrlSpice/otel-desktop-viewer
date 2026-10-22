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

test('waterfall column groups toggle together without changing span selection', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1100 })
  await page.route('**/rpc', async route => {
    const { id, method } = route.request().postDataJSON()
    await route.fulfill({
      json: {
        jsonrpc: '2.0',
        id,
        result:
          method === 'getTraceView'
            ? trace
            : method === 'searchTraceSummaries'
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
              : method === 'getStats'
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
                : [],
      },
    })
  })
  await page.goto(`/traces/${traceID}?span=${selectedID}&time=all`)
  await page.getByRole('button', { name: 'Columns', exact: true }).click()
  const picker = page.getByRole('group', { name: 'Waterfall columns' })
  const search = picker.getByRole('searchbox', { name: 'Find columns' })
  await search.fill('sequence')
  await picker
    .getByRole('checkbox', { name: 'All span attributes columns', exact: true })
    .check()
  await search.fill('')
  await expect(
    picker.getByRole('checkbox', { name: 'span: payload', exact: true })
  ).toBeChecked()
  await picker
    .getByRole('checkbox', { name: 'All span attributes columns', exact: true })
    .uncheck()
  await expect(
    picker.getByRole('checkbox', { name: 'span: sequence', exact: true })
  ).not.toBeChecked()
  await expect(page).toHaveURL(new RegExp(`span=${selectedID}`))
})

test('full gutter toggles its row with pointer and keyboard without selecting it', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1100 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  const leafID = '3333333333333333'
  const nestedTrace: JsonTraceData = {
    ...trace,
    spans: [
      ...trace.spans,
      {
        ...trace.spans[1],
        depth: 2,
        spanData: {
          ...trace.spans[1].spanData,
          spanID: leafID,
          parentSpanID: selectedID,
          name: 'leaf',
        },
      },
    ],
  }
  await page.route('**/rpc', async route => {
    const { id, method } = route.request().postDataJSON()
    const result =
      method === 'getTraceView'
        ? nestedTrace
        : method === 'searchTraceSummaries'
          ? [
              {
                traceID,
                hasRootSpan: true,
                rootSpan: { serviceName: 'checkout', name: 'other span' },
                startTime: trace.traceStart,
                durationNs: '1000000000',
                spanCount: 3,
                errorCount: 0,
              },
            ]
          : method === 'getStats'
            ? {
                storage: { sizeBytes: 0, maxSizeBytes: 0 },
                traces: {
                  traceCount: 1,
                  spanCount: 3,
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
    await route.fulfill({ json: { jsonrpc: '2.0', id, result } })
  })
  await page.goto(`/traces/${traceID}?span=${rootID}&time=all`)
  const root = page.locator(`.waterfall-row[data-span-id="${rootID}"]`)
  const branch = page.locator(`.waterfall-row[data-span-id="${selectedID}"]`)
  const leaf = page.locator(`.waterfall-row[data-span-id="${leafID}"]`)
  const toggle = branch.locator('.waterfall-row__tree-toggle')
  await expect(toggle).toHaveAccessibleName(
    'Collapse selected span, 1 child span'
  )
  await expect(toggle).toHaveCSS('height', '40px')
  await expect(toggle.locator('button')).toHaveCount(0)
  await expect(leaf.locator('.waterfall-row__tree-toggle')).toHaveCount(0)

  // The top-left corner is well outside the dot, in an ancestor's gutter column.
  await toggle.click({ position: { x: 1, y: 1 } })
  await expect(leaf).toHaveCount(0)
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await expect(root).toHaveAttribute('aria-expanded', 'true')
  await expect(root).toHaveAttribute('aria-selected', 'true')

  await toggle.press('Enter')
  await expect(leaf).toBeVisible()
  await expect(toggle).toBeFocused()
  await expect(toggle).toHaveCSS('outline-style', 'solid')
  await toggle.press('Space')
  await expect(leaf).toHaveCount(0)
  await toggle.locator('.tree-child-count').click()
  await expect(leaf).toBeVisible()
  const bounds = (await toggle.boundingBox())!
  await toggle.click({
    position: { x: bounds.width - 1, y: bounds.height - 1 },
  })
  await expect(leaf).toHaveCount(0)
  await expect(root).toHaveAttribute('aria-selected', 'true')

  await toggle.focus()
  await page.keyboard.press('Tab')
  const name = branch.getByRole('button', {
    name: 'Show span field: name in details',
  })
  await expect(name).toBeFocused()
  const accessibility = await new AxeBuilder({ page })
    .include('.waterfall-row__tree-toggle')
    .withRules([
      'button-name',
      'nested-interactive',
      'aria-valid-attr-value',
      'aria-allowed-attr',
    ])
    .analyze()
  expect(accessibility.violations).toEqual([])
  await name.click()
  await expect(branch).toHaveAttribute('aria-selected', 'true')
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
})

for (const [theme, errorColour] of [
  ['rose-pine', 'rgb(235, 111, 146)'],
  ['rose-pine-moon', 'rgb(235, 111, 146)'],
  ['rose-pine-dawn', 'rgb(180, 99, 122)'],
]) {
  test(`span status values use neutral text and an error dot only for code 2 in ${theme}`, async ({
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
      await expect(codeValue).toHaveCSS('color', neutralColour)
      const codeDot = codeField.locator('.detail-property__error-dot')
      await expect(codeDot).toHaveCount(code === 2 ? 1 : 0)
      if (code === 2) {
        await expect(codeDot).toHaveCSS('background-color', errorColour)
        await expect(
          codeField.locator('.detail-property__annotation')
        ).toHaveCSS('color', neutralColour)
      }
      if (code === 0 || code === 1) await expect(message).toHaveCount(0)
      else {
        await expect(message).toHaveText('received error message')
        await expect(message).toHaveCSS('color', neutralColour)
        const messageDot = detail.locator(
          '[data-field="status.message"] .detail-property__error-dot'
        )
        await expect(messageDot).toHaveCount(code === 2 ? 1 : 0)
        if (code === 2)
          await expect(messageDot).toHaveCSS('background-color', errorColour)
      }
      await expect(nameValue).toHaveCSS('color', neutralColour)
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
  await expect(cell.locator('.table-value-cell__inspect')).toHaveCSS(
    'text-align',
    'right'
  )
  await expect(
    row.getByRole('gridcell').first().locator('.table-value-cell__inspect')
  ).toHaveCSS('text-align', 'left')
  await expect(row).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('treegrid')).toHaveAttribute('aria-colcount', '4')
  const details = page.getByRole('region', {
    name: 'Span details',
    exact: true,
  })
  const root = page.locator(`.waterfall-row[data-span-id="${rootID}"]`)
  await page.getByRole('button', { name: 'Collapse details' }).click()
  await expect(details).toBeHidden()
  await root
    .getByRole('button', { name: 'Show resource: service.name in details' })
    .click()
  await expect(details).toBeVisible()
  await expect(
    details.locator('[data-field="service.name"][data-field-source="resource"]')
  ).toHaveClass(/detail-property--revealed/)
  await expect(root).toHaveAttribute('aria-selected', 'true')
  await details.locator('details').first().locator(':scope > summary').focus()
  await page.keyboard.press('Enter')
  await expect(details.locator('details').first()).toHaveAttribute('open')
  await cell
    .getByRole('button', { name: 'Show span: request.count in details' })
    .click()
  await expect(details.locator('details').first()).toHaveAttribute('open')
  await expect(details.locator('[data-field="request.count"]')).toHaveClass(
    /detail-property--revealed/
  )
  await expect(row).toHaveAttribute('aria-selected', 'true')
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
  const builtinKind = details.locator(
    '[data-field="kind"][data-field-kind="field"]'
  )
  const attributeKind = details.locator(
    '[data-field="kind"][data-field-kind="attribute"]'
  )
  await row
    .getByRole('button', { name: 'Show span field: kind in details' })
    .click()
  await expect(builtinKind).toHaveClass(/detail-property--revealed/)
  await expect(attributeKind).not.toHaveClass(/detail-property--revealed/)
  await row.getByRole('button', { name: 'Show span: kind in details' }).click()
  await expect(attributeKind).toHaveClass(/detail-property--revealed/)
  await expect(builtinKind).not.toHaveClass(/detail-property--revealed/)
  await page.reload()
  await expect(
    page.getByRole('columnheader', { name: 'span field: kind', exact: true })
  ).toBeVisible()
  await expect(
    page.getByRole('columnheader', { name: 'span: kind', exact: true })
  ).toBeVisible()
  await expect(row.getByRole('gridcell').nth(0)).toHaveText('2 (Server)')
  await expect(row.getByRole('gridcell').nth(1)).toHaveText('attribute kind')
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.getByRole('button', { name: 'Columns', exact: true }).click()
  await page.getByRole('searchbox', { name: 'Find columns' }).fill('')
  for (const key of ['traceId', 'spanId', 'parentSpanId']) {
    await picker
      .getByRole('checkbox', { name: `span field: ${key}`, exact: true })
      .check()
  }
  await page.keyboard.press('Escape')
  for (const [key, value] of [
    ['traceId', traceID],
    ['spanId', selectedID],
    ['parentSpanId', rootID],
  ]) {
    const button = row.getByRole('button', {
      name: `Show span field: ${key} in details`,
    })
    await button.scrollIntoViewIfNeeded()
    await expect(button).toHaveAttribute('title', value)
    await expect(
      button.locator('.identifier > [aria-hidden="true"]')
    ).toContainText(value.slice(-4))
    expect(
      await button.evaluate(element => {
        const visible = element.querySelector(
          '.identifier > [aria-hidden="true"]'
        )!
        const bounds = visible.getBoundingClientRect()
        const clip = element.getBoundingClientRect()
        return bounds.left >= clip.left && bounds.right <= clip.right
      })
    ).toBe(true)
  }
})

test('selected-span Copy and Download preserve schema URLs and exact numeric text', async ({
  page,
  context,
}) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.setViewportSize({ width: 1920, height: 1100 })
  const source = `{"resourceSpans":[{
    "schemaUrl":"https://opentelemetry.io/schemas/1.21.0",
    "resource":{"attributes":[{"key":"service.name","value":{"stringValue":"checkout"}}]},
    "scopeSpans":[{
      "schemaUrl":"https://opentelemetry.io/schemas/1.22.0",
      "scope":{"name":"instrumentation","version":"1.2"},
      "spans":[{"traceId":"${traceID}","spanId":"${selectedID}",
        "name":"selected span","startTimeUnixNano":"1700000000123456789",
        "attributes":[
          {"key":"zero","value":{"doubleValue":-0.0}},
          {"key":"large","value":{"doubleValue":18446744073709551615}},
          {"key":"exponent","value":{"doubleValue":1.2300e+40}}
        ]
      }]
    }]
  }]}\n`
  const exportURL = `/export/traces/${traceID}?spanId=${selectedID}`
  let exportRequests = 0
  await page.route(`**${exportURL}`, route => {
    exportRequests++
    return route.fulfill({
      body: source,
      contentType: 'application/json',
      headers: { 'Content-Length': String(Buffer.byteLength(source)) },
    })
  })
  await page.route('**/api/features', route => route.fulfill({ json: {} }))
  await page.route('**/rpc', route => {
    const { id, method } = route.request().postDataJSON()
    const result =
      method === 'getTraceView'
        ? trace
        : method === 'searchTraceSummaries'
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
          : method === 'getStats'
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
    return route.fulfill({
      json: {
        jsonrpc: '2.0',
        id,
        result,
      },
    })
  })
  await page.goto(`/traces/${traceID}?span=${selectedID}&time=all`)
  const detail = page.getByRole('region', { name: 'Span details', exact: true })
  const copy = detail.getByRole('button', { name: 'Copy span', exact: true })
  await expect(copy).toBeVisible()
  expect(exportRequests).toBe(0)
  await copy.press('Enter')
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(source)
  expect(exportRequests).toBe(1)
  await expect(page).toHaveURL(new RegExp(`span=${selectedID}`))
  await expect(detail.locator('details').first()).toHaveAttribute('open')
  await detail.getByRole('tab', { name: 'JSON', exact: true }).click()
  const download = detail.getByRole('link', {
    name: 'Download span',
    exact: true,
  })
  await expect(download).toHaveAttribute('href', exportURL)
  await expect(download).toHaveAttribute('download', '')
  const downloadedText = await download.evaluate(async link =>
    (await fetch(link.getAttribute('href')!)).text()
  )
  expect(downloadedText).toBe(
    await page.evaluate(() => navigator.clipboard.readText())
  )
  await page.evaluate(() =>
    navigator.clipboard.writeText('before pointer copy')
  )
  await copy.click()
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(source)
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
  const selectedExport = `{"resourceSpans":[{
    "schemaUrl":"https://opentelemetry.io/schemas/1.21.0",
    "resource":{"attributes":[{"key":"service.name","value":{"stringValue":"checkout"}}]},
    "scopeSpans":[{
      "schemaUrl":"https://opentelemetry.io/schemas/1.22.0",
      "scope":{"name":"instrumentation","version":"1.2"},
      "spans":[{
        "traceId":"${traceID}","spanId":"${selectedID}","parentSpanId":"${rootID}",
        "name":"selected span","startTimeUnixNano":"1700000000123456789",
        "attributes":[
          {"key":"sequence","value":{"intValue":"9223372036854775807"}},
          {"key":"zero","value":{"doubleValue":-0.0}},
          {"key":"large","value":{"doubleValue":18446744073709551615}},
          {"key":"payload","value":{"kvlistValue":{"values":[
            {"key":"name","value":{"stringValue":"example"}},
            {"key":"count","value":{"intValue":"42"}},
            {"key":"ready","value":{"boolValue":false}}
          ]}}}
        ]
      }]
    }]
  }]}\n`
  await page.route(`**/export/traces/${traceID}?spanId=${selectedID}`, route =>
    route.fulfill({
      body: selectedExport,
      contentType: 'application/json',
      headers: { 'Content-Length': String(Buffer.byteLength(selectedExport)) },
    })
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
  const bubble = detail.locator('[data-detail-section="span"]')
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
  const fieldHeadingBox = (await detail
    .getByRole('table', { name: 'Span fields' })
    .locator('.detail-property__header')
    .first()
    .boundingBox())!
  expect(valueBox.x).toBeCloseTo(fieldHeadingBox.x, 0)
  const palette = await page.evaluate(() => {
    const probe = document.createElement('span')
    document.body.append(probe)
    const colors = Object.fromEntries(
      ['base-content', 'secondary', 'primary', 'highlight-low'].map(token => {
        probe.style.color = `var(--color-${token})`
        return [token, getComputedStyle(probe).color]
      })
    )
    probe.remove()
    return colors
  })
  const fields = detail.getByRole('table', { name: 'Span fields' })
  const originalURL = page.url()
  for (const [fieldName, full, compact] of [
    ['traceId', traceID, '12345678…56789012'],
    ['spanId', selectedID, '2222…2222'],
    ['parentSpanId', rootID, '1111…1111'],
  ]) {
    const field = fields.locator(`[data-field="${fieldName}"]`)
    await expect(
      field.locator('.identifier > [aria-hidden="true"]')
    ).toHaveText(compact!)
    await expect(field.locator('.identifier')).toHaveAttribute('title', full!)
    const copyID = field.getByRole('button', {
      name: `Copy ${fieldName}`,
      exact: true,
    })
    await copyID.focus()
    await copyID.press('Enter')
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(full)
  }
  await expect(
    fields.getByRole('button', { name: rootID, exact: true })
  ).toBeVisible()
  await expect(page).toHaveURL(originalURL)
  await expect(
    fields.locator('[data-field="name"]').first().locator('.detail-pair__value')
  ).toHaveCSS('color', palette['base-content']!)
  await expect(
    fields
      .locator('[data-field="name"]')
      .first()
      .locator('.detail-property__type')
  ).toHaveCSS('color', palette.secondary!)
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
  const json = fields.locator('[data-field="payload"] .cm-content')
  await expect(json).toHaveAttribute('contenteditable', 'false')
  await expect(json.getByText('example', { exact: true })).toHaveCSS(
    'color',
    palette['base-content']!
  )
  await expect(json.getByText('42', { exact: true })).toHaveCSS(
    'color',
    palette.primary!
  )
  await expect(json.getByText('false', { exact: true })).toHaveCSS(
    'color',
    palette.primary!
  )
  expect(await copy.evaluate(element => element.closest('summary'))).toBeNull()
  await copy.click()
  await expect(
    bubble.locator('.detail-bubble__action').getByRole('status')
  ).toHaveText('Copied span.')
  const copiedText = await page.evaluate(() => navigator.clipboard.readText())
  expect(copiedText).toBe(selectedExport)
  const copied = JSON.parse(copiedText)
  expect(copied.resourceSpans).toHaveLength(1)
  const resource = copied.resourceSpans[0]
  expect(resource.schemaUrl).toBe('https://opentelemetry.io/schemas/1.21.0')
  expect(resource.scopeSpans[0].schemaUrl).toBe(
    'https://opentelemetry.io/schemas/1.22.0'
  )
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
  await summary.getByText('Span', { exact: true }).click()
  await expect(bubble.locator('details').first()).toHaveAttribute('open')
  await expect(copy).toBeVisible()
  const scan = await new AxeBuilder({ page })
    .include('[data-detail-section="span"]')
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
    palette['highlight-low']!
  )
})
