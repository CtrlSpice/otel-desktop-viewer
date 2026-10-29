import { routeTelemetry } from './telemetry-fixtures'
import { expect, test, type Page } from '@playwright/test'
import type { JsonTraceData } from '../src/types/wire-types'
import { THEMES, themeRgb } from './theme-colours'

const traceIDs = ['a'.repeat(32), 'b'.repeat(32)]
const spanIDs = ['1', '2', '3', '4', '5'].map(id => id.repeat(16))
const timestamp =
  BigInt(Date.parse('2026-10-24T12:00:00Z')) * 1_000_000n + 123_456_789n

function traceFixture(index: number): JsonTraceData {
  const services = ['checkout', 'database']
  return {
    traceID: traceIDs[index]!,
    traceStart: timestamp.toString(),
    unplacedSpanCount: 0,
    resources: Object.fromEntries(
      services.map((service, r) => [
        r,
        {
          attributes: [
            { key: 'service.name', value: { kind: 'string', value: service } },
          ],
          droppedAttributesCount: 0,
        },
      ])
    ),
    scopes: {
      0: {
        name: 'test',
        version: '',
        attributes: [],
        droppedAttributesCount: 0,
      },
    },
    spans: spanIDs.map((spanID, row) => ({
      depth: row === 0 ? 0 : 1,
      matched: row !== 4,
      spanData: {
        spanID,
        parentSpanID: row === 0 ? null : spanIDs[0]!,
        name: [
          'Checkout',
          'database.query',
          'database.query',
          'failed request',
          'exception request',
        ][row]!,
        traceState: '',
        flags: 1,
        kindCode: 1,
        kind: 'Internal',
        start: '0',
        dur: row === 0 ? '1269508000000' : '318000000000',
        r: index === 1 && row === 4 ? 1 : 0,
        s: 0,
        attributes: [],
        links: [],
        events:
          row === 4
            ? [
                {
                  name: 'exception',
                  timestamp: '0',
                  attributes: [],
                  droppedAttributesCount: 0,
                },
              ]
            : [],
        droppedAttributesCount: 0,
        droppedEventsCount: 0,
        droppedLinksCount: 0,
        statusCodeValue: row === 3 ? 2 : 1,
        statusCode: row === 3 ? 'Error' : 'Ok',
        statusMessage: '',
      },
    })),
  }
}

async function intercept(page: Page) {
  await page.route('**/api/features', route =>
    route.fulfill({ json: { evals: { enabled: false } } })
  )
  await routeTelemetry(page, async route => {
    const { id, method, params } = route.request().postDataJSON()
    const result =
      method === 'getTraceView'
        ? traceFixture(traceIDs.indexOf(params.traceID))
        : method === 'searchTraceSummaries'
          ? traceIDs.map((traceID, index) => ({
              traceID,
              hasRootSpan: true,
              rootSpan: {
                name:
                  index === 0 ? 'Single-service trace' : 'Multi-service trace',
                serviceName: 'checkout',
              },
              startTime: timestamp.toString(),
              durationNs: '1269508000000',
              spanCount: 5,
              errorCount: 1,
              serviceBreakdown:
                index === 0
                  ? [{ key: 'service:checkout', label: 'checkout', count: 5 }]
                  : [
                      { key: 'service:checkout', label: 'checkout', count: 4 },
                      { key: 'service:database', label: 'database', count: 1 },
                    ],
            }))
          : method === 'getStats'
            ? {
                storage: { sizeBytes: 0, maxSizeBytes: 0 },
                traces: {
                  traceCount: 2,
                  spanCount: 10,
                  serviceCount: 2,
                  errorCount: 2,
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
}

test.beforeEach(async ({ page }) => {
  await intercept(page)
  await page.setViewportSize({ width: 1920, height: 1080 })
  await page.clock.install({ time: new Date('2026-10-24T12:05:01Z') })
  await page.addInitScript(() => {
    localStorage.setItem('time-tz', 'UTC')
    localStorage.setItem('signal-drawer:open', 'true')
    localStorage.setItem('signal-drawer-width', '22')
  })
})

test('single-service waterfall dots and bars use span names while the singleton summary stays service-coloured', async ({
  page,
}) => {
  await page.goto(`/traces/${traceIDs[0]}?span=${spanIDs[0]}&time=all`)
  const card = page.getByRole('button', { name: /^Single-service trace/ })
  const summary = card.locator('.breakdown-segment__fill')
  await expect(summary).toHaveCount(1)
  const summaryColour = await summary.evaluate(
    element => getComputedStyle(element).backgroundColor
  )
  const root = page.locator(`tr[data-span-id="${spanIDs[0]}"]`)
  const operation = page.locator(`tr[data-span-id="${spanIDs[1]}"]`)
  const repeated = page.locator(`tr[data-span-id="${spanIDs[2]}"]`)
  await expect(root).toBeVisible()
  const rootColour = await root
    .locator('.span-bar')
    .evaluate(element => getComputedStyle(element).backgroundColor)
  const operationColour = await operation
    .locator('.span-bar')
    .evaluate(element => getComputedStyle(element).backgroundColor)
  expect(rootColour).not.toBe(operationColour)
  await expect(repeated.locator('.span-bar')).toHaveCSS(
    'background-color',
    operationColour
  )
  const geometry = await summary.evaluate(element => ({
    fill: element.getBoundingClientRect().width,
    track: element.closest('.breakdown-bar')!.getBoundingClientRect().width,
  }))
  expect(geometry.fill).toBeCloseTo(geometry.track, 1)
  await expect(summary).toHaveCSS('height', '8px')
  await expect(card.locator('button, a, [tabindex]')).toHaveCount(0)
  await card.locator('.breakdown-segment').hover()
  await expect(card.locator('.breakdown-tooltip:popover-open')).toHaveText(
    'checkout [5]'
  )
  await page.keyboard.press('Escape')

  for (const theme of THEMES) {
    await page.evaluate(
      id => (document.documentElement.dataset.theme = id),
      theme.id
    )
    await expect(summary).toHaveCSS('background-color', summaryColour)
    for (const [row, colour] of [
      [root, rootColour],
      [operation, operationColour],
      [repeated, operationColour],
    ] as const) {
      await expect(row.locator('.gutter__dot')).toHaveCSS(
        'background-color',
        colour
      )
      await expect(row.locator('.span-bar')).toHaveCSS(
        'background-color',
        colour
      )
      await expect(row.locator('.span-bar')).toHaveCSS('opacity', '1')
    }
    for (const spanID of spanIDs.slice(3)) {
      const row = page.locator(`tr[data-span-id="${spanID}"]`)
      await expect(row.locator('.gutter__dot')).toHaveCSS(
        'background-color',
        themeRgb(theme.id, 'error')
      )
      await expect(row.locator('.span-bar')).toHaveCSS(
        'background-color',
        themeRgb(theme.id, 'error')
      )
    }
  }
  await root.locator('.waterfall-row__tree-toggle').click()
  await expect(operation).toBeHidden()
  await expect(root.locator('.gutter__dot')).toHaveCSS(
    'background-color',
    rootColour
  )
  await root.locator('.waterfall-row__tree-toggle').click()
  await expect(operation.locator('.span-bar')).toHaveCSS(
    'background-color',
    operationColour
  )
  await page.getByRole('button', { name: /^Season:/ }).click()
  await page.getByRole('radio', { name: 'Winter' }).click()
  await page.keyboard.press('Escape')
  await expect(root.locator('.span-bar')).toHaveCSS(
    'background-color',
    rootColour
  )
  await expect(operation.locator('.span-bar')).toHaveCSS(
    'background-color',
    operationColour
  )
  await page.reload()
  await expect(root.locator('.span-bar')).toHaveCSS(
    'background-color',
    rootColour
  )
  await expect(summary).toHaveCSS('background-color', summaryColour)
})

test('multi-service waterfall colours stay service-based even when the other service is unmatched or collapsed', async ({
  page,
}) => {
  await page.goto(`/traces/${traceIDs[1]}?span=${spanIDs[0]}&time=all`)
  const card = page.getByRole('button', { name: /^Multi-service trace/ })
  const summary = card.locator('.breakdown-segment__fill').first()
  await expect(summary).toBeVisible()
  const serviceColour = await summary.evaluate(
    element => getComputedStyle(element).backgroundColor
  )
  const root = page.locator(`tr[data-span-id="${spanIDs[0]}"]`)
  for (const spanID of spanIDs.slice(0, 3)) {
    const row = page.locator(`tr[data-span-id="${spanID}"]`)
    await expect(row.locator('.span-bar')).toHaveCSS(
      'background-color',
      serviceColour
    )
    await expect(row.locator('.gutter__dot')).toHaveCSS(
      'background-color',
      serviceColour
    )
  }
  await root.locator('.waterfall-row__tree-toggle').click()
  await expect(page.locator(`tr[data-span-id="${spanIDs[1]}"]`)).toBeHidden()
  await expect(root.locator('.span-bar')).toHaveCSS(
    'background-color',
    serviceColour
  )
})

test('relative Start and long Duration share one readable row and keep the exact timestamp', async ({
  page,
}) => {
  await page.goto(`/traces/${traceIDs[0]}?span=${spanIDs[0]}&time=all`)
  const card = page.getByRole('button', { name: /^Single-service trace/ })
  const date = card.locator('.signal-date')
  await expect(date).toHaveText('Start: 5 minutes ago')
  await expect(date).toHaveAttribute(
    'title',
    /Start: 2026-10-24.*12:00:00\.123456789 UTC/
  )
  await expect(card.locator('.signal-row__date')).toHaveText(
    'Start: 5 minutes ago Duration: 21.2 minutes'
  )
  await expect(card.locator('.signal-row__meta-row')).toHaveCount(0)
  await page.clock.fastForward(60_000)
  await expect(date).toHaveText('Start: 6 minutes ago')

  for (const width of [1920, 390, 320]) {
    await page.setViewportSize({ width, height: 1080 })
    await page.evaluate(() => document.fonts.ready)
    await expect(card).toBeVisible()
    const geometry = await card
      .locator('.signal-row__date')
      .evaluate(element => {
        const [date, duration] = [...element.children]
        const left = date!.getBoundingClientRect()
        const right = duration!.getBoundingClientRect()
        return {
          left: left.toJSON(),
          right: right.toJSON(),
          overflow: element.scrollWidth > element.clientWidth,
        }
      })
    expect(geometry.overflow).toBe(false)
    expect(geometry.left.y).toBeCloseTo(geometry.right.y, 0)
    expect(geometry.right.x - geometry.left.right).toBeGreaterThanOrEqual(16)
    await expect(date).toHaveCSS('font-size', '14px')
  }

  // Longer future wording wraps as a whole group instead of clipping either value.
  await page.clock.setSystemTime(new Date('2026-10-24T11:59:10Z'))
  await page.clock.fastForward(30_000)
  await expect(date).toHaveText('Start: in less than a minute')
  const wrapped = await card.locator('.signal-row__date').evaluate(element => {
    const [date, duration] = [...element.children]
    const value = date!.lastElementChild!
    return {
      dateBottom: date!.getBoundingClientRect().bottom,
      durationTop: duration!.getBoundingClientRect().top,
      overflow: element.scrollWidth > element.clientWidth,
      clippedValue: value.scrollWidth > value.clientWidth,
    }
  })
  expect(wrapped.durationTop).toBeGreaterThanOrEqual(wrapped.dateBottom)
  expect(wrapped.overflow).toBe(false)
  expect(wrapped.clippedValue).toBe(false)
})
