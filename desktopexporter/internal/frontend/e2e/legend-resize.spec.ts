import { expect, test, type Locator, type Page } from '@playwright/test'

declare global {
  interface Window {
    legendResizeErrors: string[]
  }
}

const labels = [
  'Complete',
  'In progress',
  'Span error',
  'Internal fault',
  'Planned',
]

async function openLegend(page: Page) {
  await page.addInitScript(() => {
    window.legendResizeErrors = []
    window.addEventListener('error', event => {
      window.legendResizeErrors.push(event.message)
    })
  })
  await page.route('**/api/features', route =>
    route.fulfill({ json: { evals: { enabled: true } } })
  )
  await page.route('**/rpc', async route => {
    const { id, method, params } = route.request().postDataJSON()
    function result() {
      if (method === 'getTraceView')
        return {
          traceID: params.traceID,
          traceStart: '0',
          unplacedSpanCount: 0,
          resources: {},
          scopes: {},
          spans: [],
        }
      if (method === 'searchEvalSnapshots')
        return { snapshots: [], hasMore: false }
      if (method === 'getStats')
        return {
          storage: { sizeBytes: 0, maxSizeBytes: 0 },
          traces: {
            traceCount: 0,
            spanCount: 0,
            serviceCount: 0,
            errorCount: 0,
            lastReceived: null,
          },
          logs: { logCount: 0, errorCount: 0, lastReceived: null },
          metrics: { metricCount: 0, dataPointCount: 0, lastReceived: null },
          rejections: [],
        }
      return []
    }
    await route.fulfill({ json: { jsonrpc: '2.0', id, result: result() } })
  })
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.goto('/evals?run=direct&row=case%2Frequest-latency')
  const legend = page.getByRole('list', { name: 'Execution status legend' })
  await expect(legend).toBeVisible()
  await page.evaluate(() => document.fonts.ready)
  return legend
}

async function rows(legend: Locator) {
  return legend
    .locator('.legend-rows__row')
    .evaluateAll(elements =>
      elements.map(row =>
        Array.from(row.querySelectorAll('[role="listitem"]'), entry =>
          entry.textContent!.trim()
        )
      )
    )
}

async function expectStableLayout(legend: Locator) {
  await expect(legend.getByRole('listitem')).toHaveText(labels)
  await expect
    .poll(() =>
      legend.evaluate(async element => {
        const samples: string[] = []
        for (let frame = 0; frame < 8; frame++) {
          await new Promise(requestAnimationFrame)
          samples.push(
            JSON.stringify(
              [element, ...element.querySelectorAll('[role="listitem"]')].map(
                node => {
                  const rect = node.getBoundingClientRect()
                  return [rect.x, rect.y, rect.width, rect.height]
                }
              )
            )
          )
        }
        return new Set(samples).size
      })
    )
    .toBe(1)
  const geometry = await legend.evaluate(element => {
    const bounds = element.getBoundingClientRect()
    return Array.from(element.querySelectorAll('[role="listitem"]'), entry => {
      const rect = entry.getBoundingClientRect()
      return {
        wholeEntryFits:
          rect.left >= bounds.left &&
          (rect.right <= bounds.right + 1 ||
            (rect.width > bounds.width &&
              entry.parentElement!.children.length === 1)),
        height: rect.height,
        fontSize: getComputedStyle(entry).fontSize,
      }
    })
  })
  for (const entry of geometry) {
    expect(entry).toEqual({
      wholeEntryFits: true,
      height: 24,
      fontSize: '14px',
    })
  }
}

test('legend resizing and eval view changes settle without observer feedback', async ({
  page,
}) => {
  const legend = await openLegend(page)
  await expectStableLayout(legend)
  const wideRows = await rows(legend)
  await page.getByRole('radio', { name: 'Tool calls', exact: true }).click()
  await page.getByRole('radio', { name: 'Results', exact: true }).click()
  await page.setViewportSize({ width: 390, height: 844 })
  await expectStableLayout(legend)
  expect((await rows(legend)).length).toBeGreaterThan(wideRows.length)
  await page.setViewportSize({ width: 1440, height: 1000 })
  await expectStableLayout(legend)
  await expect.poll(() => rows(legend)).toEqual(wideRows)
  expect(await page.evaluate(() => window.legendResizeErrors)).toEqual([])
})

test('measured legend entries stay whole and rebalance the last pair at resize boundaries', async ({
  page,
}) => {
  const legend = await openLegend(page)
  const { widths, gap } = await legend.evaluate(element => ({
    widths: Array.from(
      element.querySelectorAll('[role="listitem"]'),
      entry => entry.getBoundingClientRect().width
    ),
    gap: parseFloat(getComputedStyle(element).columnGap),
  }))
  const four = Math.ceil(widths.slice(0, 4).reduce((a, b) => a + b, gap * 3))
  const all = Math.ceil(widths.reduce((a, b) => a + b, gap * 4))
  for (const [width, expected] of [
    [four, [labels.slice(0, 3), labels.slice(3)]],
    [all, [labels]],
    [four, [labels.slice(0, 3), labels.slice(3)]],
  ] as const) {
    await legend.evaluate((element, width) => {
      element.style.width = `${width}px`
      element.style.flexShrink = '0'
    }, width)
    await expect.poll(() => rows(legend)).toEqual(expected)
    await expectStableLayout(legend)
  }
  expect(await page.evaluate(() => window.legendResizeErrors)).toEqual([])
})
