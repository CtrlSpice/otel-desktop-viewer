import { expect, test, type Page, type Route } from '@playwright/test'
import type {
  JsonAggregateBucket,
  JsonDataPoint,
  JsonMetricSeriesViewData,
  JsonMetricSummary,
  JsonMetricViewData,
} from '../src/types/wire-types'

const startMs = 1_700_000_000_000
const startNs = String(BigInt(startMs) * 1_000_000n)
const endNs = String(BigInt(startMs + 40_000) * 1_000_000n)
const resource = {
  attributes: [
    {
      key: 'service.name',
      value: { kind: 'string' as const, value: 'checkout-api' },
    },
  ],
  droppedAttributesCount: 0,
}
const timeseries: JsonMetricSeriesViewData[] = ['first', 'second'].map(
  name => ({
    seriesRef: name,
    attributes: [
      { key: 'route', value: { kind: 'string', value: `/${name}` } },
    ],
    resource,
    datapoints: Array.from({ length: 40 }, (_, index) => ({
      id: `${name}-${index}`,
      metricType: 'Gauge',
      timestamp: String(BigInt(startMs + index * 1000) * 1_000_000n),
      timestampMs: startMs + index * 1000,
      startTime: startNs,
      flags: 0,
      exemplars: [],
      doubleValue: index + 1,
      intValue: null,
      valueType: 'Double',
    })),
    stats: null,
    datapointCount: 40,
    lastSeenNs: endNs,
    rateStats: null,
    views: null,
    sparkline: null,
  })
)
const metric: JsonMetricViewData = {
  metricRef: 'series-layout',
  name: 'request.duration',
  description: '',
  metadata: [],
  unit: 'ms',
  metricType: 'Gauge',
  aggregationTemporalityCode: null,
  aggregationTemporality: null,
  isMonotonic: null,
  resourceDroppedAttributesCount: 0,
  resourceSchemaUrl: '',
  resource,
  scopeName: '',
  scopeVersion: '',
  scopeSchemaUrl: '',
  scopeDroppedAttributesCount: 0,
  scope: { name: '', version: '', attributes: [], droppedAttributesCount: 0 },
  timeseries,
  aggregate: null,
  scalarAggregate: null,
  datapointCount: 80,
  boundsMismatch: null,
  lastSeenNs: endNs,
  window: {
    requested: { startNs: null, endNs: null },
    effective: { startNs, endNs },
  },
}
const summary: JsonMetricSummary = {
  metricRef: metric.metricRef,
  name: metric.name,
  description: '',
  unit: metric.unit,
  metricType: 'Gauge',
  aggregationTemporalityCode: null,
  aggregationTemporality: null,
  isMonotonic: null,
  serviceName: 'checkout-api',
  seriesCount: 2,
  seriesCardinality: 2,
  dataPointCount: 80,
  lastValue: 40,
  lastSeen: endNs,
}

test.beforeEach(async ({ page }) => {
  await page.route('**/rpc', async route => {
    const request = route.request().postDataJSON() as {
      id: number
      method: string
    }
    const result =
      request.method === 'getMetricView'
        ? metric
        : request.method === 'searchMetricSummaries'
          ? [summary]
          : request.method === 'getMetricAggregateView'
            ? { aggregate: null, scalarAggregate: null }
            : request.method === 'getStats'
              ? {
                  storage: { sizeBytes: 0, maxSizeBytes: 0 },
                  traces: {
                    traceCount: 0,
                    spanCount: 0,
                    serviceCount: 0,
                    errorCount: 0,
                    lastReceived: null,
                  },
                  logs: { logCount: 0, errorCount: 0, lastReceived: null },
                  metrics: {
                    metricCount: 1,
                    dataPointCount: 80,
                    lastReceived: null,
                  },
                  rejections: [],
                }
              : []
    await route.fulfill({ json: { jsonrpc: '2.0', id: request.id, result } })
  })
})

test.describe('independent histogram aggregate loading', () => {
  const bucket: JsonAggregateBucket = {
    timestamp: startNs,
    startTime: startNs,
    count: 12,
    sum: 18,
    min: 0,
    max: 4,
    quantiles: { '0.5': 1.5, '0.95': 1.9, '0.99': 2 },
    explicitBounds: [1, 2],
    bucketCounts: [2, 7, 3],
  }

  async function holdAggregates(page: Page) {
    const pending: { route: Route; wholeWindow: boolean }[] = []
    const histogramMetric: JsonMetricViewData = {
      ...metric,
      metricType: 'Histogram',
      aggregationTemporalityCode: 1,
      aggregationTemporality: 'Delta',
      timeseries: timeseries.map(series => ({
        ...series,
        datapoints: [
          {
            ...series.datapoints[0]!,
            metricType: 'Histogram',
            aggregationTemporalityCode: 1,
            aggregationTemporality: 'Delta',
            count: '12',
            sum: 18,
            min: 0,
            max: 4,
            explicitBounds: [1, 2],
            bucketCounts: ['2', '7', '3'],
            quantiles: bucket.quantiles,
          },
        ],
      })),
    }
    await page.route('**/rpc', async route => {
      const request = route.request().postDataJSON()
      if (request.method === 'getMetricAggregateView') {
        pending.push({ route, wholeWindow: request.params.targetBuckets === 1 })
        return
      }
      const result =
        request.method === 'getMetricView'
          ? histogramMetric
          : request.method === 'searchMetricSummaries'
            ? [
                {
                  ...summary,
                  metricType: 'Histogram',
                  aggregationTemporalityCode: 1,
                  aggregationTemporality: 'Delta',
                },
              ]
            : undefined
      if (result === undefined) return route.fallback()
      await route.fulfill({ json: { jsonrpc: '2.0', id: request.id, result } })
    })
    await page.goto(
      `/metrics/${metric.metricRef}?time=all&htab=heatmap&hscope=window`
    )
    await expect.poll(() => pending.length).toBe(2)
    return pending
  }

  async function respond(route: Route, value = bucket, fail = false) {
    await route.fulfill({
      json: {
        jsonrpc: '2.0',
        id: route.request().postDataJSON().id,
        ...(fail
          ? { error: { code: -32603, message: 'Aggregate unavailable' } }
          : { result: { aggregate: [value], scalarAggregate: null } }),
      },
    })
  }

  for (const fail of [false, true]) {
    test(`heatmap renders before the window summary ${fail ? 'fails' : 'arrives'}`, async ({
      page,
    }) => {
      const pending = await holdAggregates(page)
      await respond(pending.find(request => !request.wholeWindow)!.route)
      await expect(page.locator('.heatmap-cell')).toHaveCount(3)
      await expect(page.locator('.heatmap-cell').first()).toBeVisible()
      const paths = await page
        .locator('.heatmap-blob-fill')
        .evaluateAll(elements => elements.map(el => el.getAttribute('d')))
      await respond(
        pending.find(request => request.wholeWindow)!.route,
        bucket,
        fail
      )
      await page.getByRole('tab', { name: 'Histogram', exact: true }).click()
      if (fail)
        await expect(
          page.getByText('No aggregate in range', { exact: true })
        ).toBeVisible()
      else await expect(page.locator('.lc-bars-bar')).toHaveCount(3)
      await page.getByRole('tab', { name: 'Heatmap', exact: true }).click()
      await expect(page.locator('.heatmap-cell')).toHaveCount(3)
      expect(
        await page
          .locator('.heatmap-blob-fill')
          .evaluateAll(elements => elements.map(el => el.getAttribute('d')))
      ).toEqual(paths)
    })
  }

  test('window summary renders while time buckets are pending', async ({
    page,
  }) => {
    const pending = await holdAggregates(page)
    await respond(pending.find(request => request.wholeWindow)!.route)
    await page.getByRole('tab', { name: 'Histogram', exact: true }).click()
    await expect(page.locator('.lc-bars-bar')).toHaveCount(3)
    await respond(
      pending.find(request => !request.wholeWindow)!.route,
      bucket,
      true
    )
    await expect(page.locator('.lc-bars-bar')).toHaveCount(3)
  })

  for (const fail of [false, true]) {
    test(`late aggregate ${fail ? 'errors' : 'results'} cannot replace a newer checked selection`, async ({
      page,
    }) => {
      const pending = await holdAggregates(page)
      await page
        .getByRole('checkbox', {
          name: 'Show route: /second in chart',
          exact: true,
        })
        .uncheck()
      await expect.poll(() => pending.length).toBe(4)
      for (const request of pending.slice(2)) await respond(request.route)
      await expect(page.locator('.heatmap-cell')).toHaveCount(3)
      const finished = Promise.all(
        pending
          .slice(0, 2)
          .map(({ route }) =>
            page
              .waitForResponse(
                response => response.request() === route.request()
              )
              .then(response => response.finished())
          )
      )
      for (const request of pending.slice(0, 2)) {
        await respond(
          request.route,
          { ...bucket, explicitBounds: [2], bucketCounts: [4, 8] },
          fail
        )
      }
      await finished
      await page.evaluate(
        () =>
          new Promise(resolve =>
            requestAnimationFrame(() => requestAnimationFrame(resolve))
          )
      )
      await expect(page.locator('.heatmap-cell')).toHaveCount(3)
      await page.getByRole('tab', { name: 'Histogram', exact: true }).click()
      await expect(page.locator('.lc-bars-bar')).toHaveCount(3)
    })
  }
})

for (const [theme, hoverColour, selectedColour, focusColour] of [
  ['rose-pine', 'rgb(33, 32, 46)', 'rgb(64, 61, 82)', 'rgb(196, 167, 231)'],
  [
    'rose-pine-moon',
    'rgb(42, 40, 62)',
    'rgb(68, 65, 90)',
    'rgb(196, 167, 231)',
  ],
  [
    'rose-pine-dawn',
    'rgb(244, 237, 232)',
    'rgb(223, 218, 217)',
    'rgb(144, 122, 169)',
  ],
]) {
  for (const metricType of ['Histogram', 'ExponentialHistogram'] as const) {
    test(`${metricType} graph highlights use rounded neutral interaction roles in ${theme}`, async ({
      page,
    }) => {
      const datapoints: JsonDataPoint[] = [0, 1].map(index => {
        const common = {
          id: `hist-${index}`,
          timestamp: String(BigInt(startMs + index * 1000) * 1_000_000n),
          timestampMs: startMs + index * 1000,
          startTime: startNs,
          flags: 0,
          exemplars: [],
          count: '12',
          sum: 18,
          min: 0,
          max: 4,
          quantiles: { '0.5': 100, '0.95': 200, '0.99': 300 },
          aggregationTemporalityCode: 1,
          aggregationTemporality: 'Delta',
        }
        return metricType === 'Histogram'
          ? {
              ...common,
              metricType,
              explicitBounds: [1, 2],
              bucketCounts: ['2', '7', '3'],
            }
          : {
              ...common,
              metricType,
              scale: 0,
              zeroCount: '2',
              zeroThreshold: 0.1,
              positiveBucketOffset: 0,
              positiveBucketCounts: ['7', '3'],
              negativeBucketOffset: 0,
              negativeBucketCounts: [],
            }
      })
      const aggregate: JsonAggregateBucket[] = datapoints.map(point => ({
        timestamp: point.timestamp,
        startTime: startNs,
        count: 12,
        sum: 18,
        min: 0,
        max: 4,
        quantiles: { '0.5': 1.5, '0.95': 1.9, '0.99': 2 },
        ...(point.metricType === 'Histogram'
          ? { explicitBounds: [1, 2], bucketCounts: [2, 7, 3] }
          : {
              scale: 0,
              zeroCount: 2,
              zeroThreshold: 0.1,
              positiveBucketOffset: 0,
              positiveBucketCounts: [7, 3],
              negativeBucketOffset: 0,
              negativeBucketCounts: [],
            }),
      }))
      const histogramMetric: JsonMetricViewData = {
        ...metric,
        metricType,
        aggregationTemporalityCode: 1,
        aggregationTemporality: 'Delta',
        datapointCount: 2,
        timeseries: [{ ...timeseries[0]!, datapoints, datapointCount: 2 }],
      }
      await page.route('**/rpc', async route => {
        const request = route.request().postDataJSON() as {
          id: number
          method: string
        }
        const result =
          request.method === 'getMetricView'
            ? histogramMetric
            : request.method === 'getMetricAggregateView'
              ? { aggregate, scalarAggregate: null }
              : request.method === 'searchMetricSummaries'
                ? [
                    {
                      ...summary,
                      metricType,
                      aggregationTemporalityCode: 1,
                      aggregationTemporality: 'Delta',
                      seriesCount: 1,
                      seriesCardinality: 1,
                      dataPointCount: 2,
                    },
                  ]
                : undefined
        if (result === undefined) return route.fallback()
        await route.fulfill({
          json: { jsonrpc: '2.0', id: request.id, result },
        })
      })
      await page.addInitScript(
        theme => localStorage.setItem('theme', theme),
        theme
      )
      await page.setViewportSize({ width: 1440, height: 1000 })
      await page.emulateMedia({ reducedMotion: 'reduce' })

      for (const [
        tab,
        markSelector,
        hoverSelector,
        selectionSelector,
        keyboardSelector,
        chartLabel,
      ] of [
        [
          'histogram',
          '.lc-bars-bar',
          '.histogram-hover-column',
          '.value-bucket-pin-highlight',
          '.chart-keyboard-cursor-band',
          'Histogram distribution chart',
        ],
        [
          'heatmap',
          '.heatmap-cell',
          '.heatmap-hover-column',
          '.heatmap-selection',
          '.chart-keyboard-cursor-cell',
          'Histogram heatmap',
        ],
      ]) {
        await page.goto(
          `/metrics/${metric.metricRef}?htab=${tab}&hscope=window`
        )
        const mark = page.locator(markSelector).first()
        await expect(mark).toBeVisible()
        await expect(
          page.locator('.pane-header .chart-stat-overlays-toggle')
        ).toHaveCount(1)
        await expect(
          page.locator('.metric-chart-view .chart-stat-overlays-toggle')
        ).toHaveCount(0)
        if (tab === 'heatmap') {
          // Each row has the same colour in both adjacent time columns.
          // Render three connected outlines while retaining all six source cells.
          const blobs = page.locator('.heatmap-blob')
          await expect(blobs).toHaveCount(3)
          for (const blob of await blobs.all()) {
            await expect(blob.locator('.heatmap-cell')).toHaveCount(2)
            await expect(blob.locator('.heatmap-blob-fill')).toHaveCount(1)
            await expect(blob.locator('.heatmap-cell').first()).toHaveCSS(
              'fill-opacity',
              '0'
            )
            const clipped = await blob.evaluate(element => {
              const id = element.getAttribute('clip-path')!.slice(5, -1)
              return (
                document
                  .getElementById(id)
                  ?.querySelector('path')
                  ?.getAttribute('d') ?? ''
              )
            })
            expect(clipped).toContain('Q')
            expect(clipped.match(/M/g)).toHaveLength(1)
          }
          const legend = page.getByRole('list', { name: 'Heatmap count scale' })
          await expect(legend).toBeVisible()
          const legendBox = (await page
            .locator('.heatmap-legend')
            .boundingBox())!
          const chartBox = (await page
            .locator('.metric-heatmap-chart')
            .boundingBox())!
          expect(legendBox.y + legendBox.height).toBeLessThanOrEqual(chartBox.y)
          const titleColour = await page
            .locator('.pane-header__title')
            .first()
            .evaluate(title => getComputedStyle(title).color)
          await expect(legend).toHaveCSS('color', titleColour)
          await expect(
            legend.locator('.heatmap-legend__swatch').first()
          ).toHaveCSS('width', '16px')
          expect(await legend.innerText()).not.toMatch(/[()[\]]/)
        }
        const dataFill = await mark.evaluate(
          element => getComputedStyle(element).fill
        )
        const box = (await mark.boundingBox())!
        await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
        const hover = page.locator(hoverSelector)
        await expect(hover).toBeVisible()
        await expect(hover).toHaveCSS('fill', hoverColour)
        await expect(hover).toHaveCSS('stroke', 'none')
        await expect(hover).toHaveCSS('rx', '8px')
        await expect(hover).toHaveCSS('pointer-events', 'none')
        let hoveredFill = dataFill
        if (tab === 'heatmap') {
          await expect(mark).toHaveClass(/heatmap-cell--hovered/)
          await expect(mark).toHaveCSS('fill-opacity', '1')
          await expect(page.locator('.heatmap-cell--hovered')).toHaveCount(1)
          expect(
            await hover.evaluate(element => {
              const blobs = document.querySelector('.heatmap-blobs')!
              return Boolean(
                blobs.compareDocumentPosition(element) &
                Node.DOCUMENT_POSITION_FOLLOWING
              )
            })
          ).toBe(true)
          hoveredFill = await mark.evaluate(
            element => getComputedStyle(element).fill
          )
          expect(hoveredFill).not.toBe(dataFill)
        }
        await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2)
        const selection = page.locator(selectionSelector)
        await expect(selection).toBeVisible()
        await expect(selection).toHaveCSS('fill', selectedColour)
        await expect(selection).toHaveCSS('stroke', selectedColour)
        await expect(selection).toHaveCSS('fill-opacity', '0.28')
        await expect(selection).toHaveCSS('rx', '8px')
        await expect(selection).toHaveCSS('pointer-events', 'none')
        if (tab === 'heatmap') {
          await expect(mark).toHaveClass(/heatmap-cell--selected/)
          const selectedFill = await mark.evaluate(
            element => getComputedStyle(element).fill
          )
          expect(selectedFill).not.toBe(dataFill)
          expect(selectedFill).not.toBe(hoveredFill)
          const card = page.locator('.metric-heatmap-chart .chart-sticky__card')
          await expect(card).toBeVisible()
          const cellDetails = card.locator('.heatmap-selected-cell')
          await expect(cellDetails.locator('dd').nth(1)).toHaveText('2')
          await expect(
            card.locator('.chart-selection-legend__value').first()
          ).toHaveText('12')
          await expect
            .poll(async () => {
              const cardBox = (await card.boundingBox())!
              return Math.abs(
                cardBox.y + cardBox.height / 2 - (box.y + box.height / 2)
              )
            })
            .toBeLessThan(1)
          await expect
            .poll(async () => {
              const cardBox = (await card.boundingBox())!
              const columnBox = (await selection.boundingBox())!
              const plot = await page
                .locator('.heatmap-cell')
                .evaluateAll(cells => {
                  const boxes = cells.map(cell => cell.getBoundingClientRect())
                  return {
                    left: Math.min(...boxes.map(box => box.left)),
                    right: Math.max(...boxes.map(box => box.right)),
                  }
                })
              const rightGap = cardBox.x - (columnBox.x + columnBox.width)
              const leftGap = columnBox.x - (cardBox.x + cardBox.width)
              const roomBeside =
                plot.right - columnBox.x - columnBox.width >=
                  cardBox.width + 12 ||
                columnBox.x - plot.left >= cardBox.width + 12
              if (roomBeside)
                return (
                  Math.min(Math.abs(rightGap - 12), Math.abs(leftGap - 12)) < 1
                )
              // Narrow panes keep the card over its column, inside the plot.
              return (
                cardBox.x >= plot.left - 1 &&
                cardBox.x + cardBox.width <= plot.right + 1 &&
                rightGap <= 0 &&
                leftGap <= 0
              )
            })
            .toBe(true)
          // A different row in the same column updates the picked cell,
          // rather than toggling the column pin off.
          const x = await mark.getAttribute('x')
          const y = await mark.getAttribute('y')
          const nextCell = page
            .locator(`.heatmap-cell[x="${x}"]:not([y="${y}"])`)
            .first()
          const nextBox = (await nextCell.boundingBox())!
          await page.mouse.click(
            nextBox.x + nextBox.width / 2,
            nextBox.y + nextBox.height / 2
          )
          await expect(selection).toBeVisible()
          await expect(cellDetails.locator('dd').nth(1)).toHaveText('7')
          await expect
            .poll(async () => {
              const cardBox = (await card.boundingBox())!
              return Math.abs(
                cardBox.y +
                  cardBox.height / 2 -
                  (nextBox.y + nextBox.height / 2)
              )
            })
            .toBeLessThan(1)
        } else {
          await expect(mark).not.toHaveCSS('fill', dataFill)
        }
        const keyboard = page.getByRole('application', {
          name: chartLabel,
          exact: true,
        })
        await keyboard.focus()
        await keyboard.press('ArrowRight')
        await expect(page.locator(keyboardSelector)).toHaveCSS(
          'stroke',
          focusColour
        )
        await expect(page.locator(keyboardSelector)).toHaveCSS('rx', '8px')
        if (tab === 'heatmap') {
          await keyboard.press('Control+Home')
          await keyboard.press('Enter')
          await expect(
            page.locator('.heatmap-selected-cell dd').nth(1)
          ).toHaveText('3')
        }
        await keyboard.press('Escape')
        await expect(selection).toHaveCount(0)
        if (tab === 'heatmap') {
          await page.mouse.move(0, 0)
          await expect(mark).not.toHaveClass(
            /heatmap-cell--hovered|heatmap-cell--selected/
          )
          await expect(mark).toHaveCSS('fill', dataFill)
          await expect(mark).toHaveCSS('fill-opacity', '0')
        } else {
          await page.mouse.move(0, 0)
          await keyboard.evaluate(element => (element as HTMLElement).blur())
          await expect(mark).toHaveCSS('fill', dataFill)
        }
        const quantileLines = page.locator(
          tab === 'heatmap' ? '.heatmap-quantile-line' : '.quantile-line'
        )
        const quantileColours =
          theme === 'rose-pine-dawn'
            ? ['rgb(40, 105, 131)', 'rgb(234, 157, 52)', 'rgb(180, 99, 122)']
            : [
                theme === 'rose-pine'
                  ? 'rgb(49, 116, 143)'
                  : 'rgb(62, 143, 176)',
                'rgb(246, 193, 119)',
                'rgb(235, 111, 146)',
              ]
        await expect(quantileLines).toHaveCount(3)
        for (const [index, line] of (await quantileLines.all()).entries()) {
          await expect(line).toHaveCSS('stroke', quantileColours[index]!)
          await expect(line).toHaveCSS('stroke-width', '2px')
          await expect(line).toHaveCSS('opacity', '0.35')
          await expect(line).toHaveCSS('pointer-events', 'none')
        }
        expect(
          await quantileLines.evaluateAll(lines =>
            lines.map(line => getComputedStyle(line).strokeDasharray)
          )
        ).toEqual(['6px, 4px', '6px, 4px', '6px, 4px'])
        if (tab === 'heatmap') {
          const overlays = page.locator('.heatmap-quantile-line')
          await expect(overlays).toHaveCount(3)
          const channels = page.locator('.heatmap-quantile-channel')
          await expect(channels).toHaveCount(3)
          expect(
            await channels.evaluateAll(paths =>
              paths.map(path => path.getAttribute('d'))
            )
          ).toEqual(
            await overlays.evaluateAll(paths =>
              paths.map(path => path.getAttribute('d'))
            )
          )
          for (const channel of await channels.all()) {
            await expect(channel).toHaveAttribute('stroke-width', '4')
          }
          await expect(page.locator('.heatmap-blobs')).toHaveAttribute(
            'mask',
            /^url\(#.+\)$/
          )
          for (const line of await overlays.all()) {
            await expect(line).toHaveCSS('fill', 'none')
            await expect(line).toHaveCSS('pointer-events', 'none')
            // Each estimate stays inside its original populated bucket's
            // geometry; visual channels do not change the data coordinates.
            expect(
              await line.evaluate(element => {
                const path = element as SVGPathElement
                const point = path.getPointAtLength(path.getTotalLength() / 4)
                const screen = new DOMPoint(point.x, point.y).matrixTransform(
                  path.getScreenCTM()!
                )
                return [...document.querySelectorAll('.heatmap-cell')].some(
                  cell => {
                    const box = cell.getBoundingClientRect()
                    return (
                      screen.x >= box.left - 0.5 &&
                      screen.x <= box.right + 0.5 &&
                      screen.y >= box.top - 0.5 &&
                      screen.y <= box.bottom + 0.5
                    )
                  }
                )
              })
            ).toBe(true)
          }
          await expect(
            page.getByRole('tab', { name: 'Quantiles', exact: true })
          ).toHaveCount(0)
        }
        const countLegend = page.getByRole('list', {
          name:
            tab === 'histogram'
              ? 'Histogram count scale'
              : 'Heatmap count scale',
          exact: true,
        })
        await expect(countLegend).toBeVisible()
        const quantileLabels = page.locator(
          tab === 'heatmap' ? '.heatmap-quantile-label' : '.series-stat-tooltip'
        )
        await expect(quantileLabels).toHaveCount(3)
        const overlaysToggle = page.getByRole('checkbox', {
          name: 'Show chart stat overlays',
          exact: true,
        })
        await overlaysToggle.uncheck()
        await expect(quantileLines).toHaveCount(0)
        await expect(quantileLabels).toHaveCount(0)
        await expect(page.locator('.heatmap-quantile-channel')).toHaveCount(0)
        await expect(countLegend).toBeVisible()
        await overlaysToggle.check()
        await expect(quantileLines).toHaveCount(3)
      }
      await page.goto(
        `/metrics/${metric.metricRef}?htab=quantiles&hscope=window&series=first&dp=hist-0`
      )
      await expect(
        page.getByRole('tab', { name: 'Heatmap', exact: true })
      ).toHaveAttribute('aria-selected', 'true')
      await expect(page.locator('.heatmap-quantile-line')).toHaveCount(3)
      expect(new URL(page.url()).searchParams.get('dp')).toBe('hist-0')
      if (theme === 'rose-pine-moon' && metricType === 'Histogram') {
        await page
          .getByRole('button', { name: 'Collapse sidebar', exact: true })
          .click()
        await page
          .getByRole('button', { name: 'Collapse details', exact: true })
          .click()
        await page
          .getByRole('checkbox', {
            name: 'Show chart stat overlays',
            exact: true,
          })
          .uncheck()
        await page.setViewportSize({ width: 390, height: 1000 })
        await expect(
          page.getByRole('checkbox', {
            name: 'Show chart stat overlays',
            exact: true,
          })
        ).not.toBeChecked()
        await expect(
          page.locator('.pane-header').getByRole('checkbox', {
            name: 'Show chart stat overlays',
            exact: true,
          })
        ).toBeVisible()
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= window.innerWidth
          )
        ).toBe(true)
        expect(new URL(page.url()).searchParams.get('dp')).toBe('hist-0')
      }
    })
  }
}

test('extrema use the series colour and the average sits behind the series without a data dot', async ({
  page,
}) => {
  await page.route('**/rpc', async route => {
    const request = route.request().postDataJSON() as {
      id: number
      method: string
    }
    if (request.method !== 'getMetricView') return route.fallback()
    const withStats: JsonMetricViewData = {
      ...metric,
      timeseries: metric.timeseries.map(series => ({
        ...series,
        stats: { count: 40, min: 1, max: 40, sum: 820, avg: 20.5 },
      })),
    }
    await route.fulfill({
      json: { jsonrpc: '2.0', id: request.id, result: withStats },
    })
  })
  await page.goto(
    `/metrics/${metric.metricRef}?agg=raw&series=first&dp=first-10`
  )
  const average = page.locator('.series-average-marker')
  await expect(average).toBeAttached()
  await expect(average.locator('circle')).toHaveCount(0)
  const averageLabel = page.locator('.series-stat-tooltip--right')
  await expect(
    averageLabel.locator('.chart-selection-legend__label')
  ).toHaveText('avg')
  await expect(
    averageLabel.locator('.chart-selection-legend__value')
  ).toHaveText('20.5')
  await expect(
    averageLabel.locator('.chart-selection-legend--stat')
  ).toBeVisible()
  await expect(average.locator('.series-average-line')).toHaveCSS(
    'stroke-dasharray',
    '4px, 3px'
  )
  await expect(average.locator('.series-average-line')).toHaveCSS(
    'opacity',
    '0.35'
  )
  const drawing = await page
    .locator('.metric-time-series-chart')
    .evaluate(chart => {
      const min = [...chart.querySelectorAll('.series-stat-marker')].find(
        marker => marker.querySelector('title')?.textContent?.startsWith('min ')
      )!
      const max = [...chart.querySelectorAll('.series-stat-marker')].find(
        marker => marker.querySelector('title')?.textContent?.startsWith('max ')
      )!
      const minColour = getComputedStyle(min.querySelector('circle')!).fill
      const maxColour = getComputedStyle(max.querySelector('circle')!).fill
      const series = [...chart.querySelectorAll('path')].find(
        path => getComputedStyle(path).stroke === minColour
      )
      const reference = chart.querySelector('.series-average-line')!
      const label = chart.querySelector(
        '.series-stat-tooltip--right .chart-selection-legend__dot'
      )!
      const labelBox = chart
        .querySelector('.series-stat-tooltip--right')!
        .getBoundingClientRect()
      const lineBox = reference.getBoundingClientRect()
      return {
        extremaMatch: minColour === maxColour,
        seriesFound: !!series,
        averageBehindSeries:
          !!series &&
          !!(
            reference.compareDocumentPosition(series) &
            Node.DOCUMENT_POSITION_FOLLOWING
          ),
        averageHasOwnColour: getComputedStyle(reference).stroke !== minColour,
        averageLabelMatches:
          getComputedStyle(reference).stroke ===
          getComputedStyle(label).backgroundColor,
        averageMatchesAxis:
          getComputedStyle(reference).stroke ===
          getComputedStyle(chart.querySelector('.lc-axis-tick-label')!).fill,
        averageLabelAtRight: Math.abs(labelBox.right - lineBox.right) < 1,
      }
    })
  expect(drawing).toEqual({
    extremaMatch: true,
    seriesFound: true,
    averageBehindSeries: true,
    averageHasOwnColour: true,
    averageLabelMatches: true,
    averageMatchesAxis: true,
    averageLabelAtRight: true,
  })
})

test('a new single-point cumulative counter opens in Raw without preferences', async ({
  page,
}) => {
  const point = {
    ...timeseries[0]!.datapoints[0]!,
    metricType: 'Sum' as const,
    isMonotonic: true,
    aggregationTemporalityCode: 2,
    aggregationTemporality: 'Cumulative',
  }
  const counter = {
    ...metric,
    metricType: 'Sum',
    isMonotonic: true,
    aggregationTemporalityCode: 2,
    aggregationTemporality: 'Cumulative',
    datapointCount: 1,
    timeseries: [{ ...timeseries[0]!, datapoints: [point], datapointCount: 1 }],
  }
  await page.route('**/rpc', async route => {
    const request = route.request().postDataJSON() as {
      id: number
      method: string
    }
    if (request.method === 'getMetricView') {
      await route.fulfill({
        json: { jsonrpc: '2.0', id: request.id, result: counter },
      })
    } else if (request.method === 'searchMetricSummaries') {
      await route.fulfill({
        json: {
          jsonrpc: '2.0',
          id: request.id,
          result: [
            {
              ...summary,
              metricType: 'Sum',
              isMonotonic: true,
              aggregationTemporalityCode: 2,
              aggregationTemporality: 'Cumulative',
              seriesCount: 1,
              seriesCardinality: 1,
              dataPointCount: 1,
            },
          ],
        },
      })
    } else await route.fallback()
  })
  await page.goto(`/metrics/${metric.metricRef}`)
  await expect(
    page.getByRole('tab', { name: 'Raw', exact: true })
  ).toHaveAttribute('aria-selected', 'true')
  await expect(
    page.getByRole('tab', { name: 'Rate', exact: true })
  ).toHaveAttribute('aria-selected', 'false')
  await expect(page.locator('.metric-time-series-chart')).toBeVisible()
  await expect(
    page.getByText('No datapoints in selected timeseries', { exact: true })
  ).toHaveCount(0)
  await expect(
    page
      .getByRole('region', { name: 'Metric chart', exact: true })
      .locator('[aria-label="Chart time range"]')
  ).toBeVisible()
})

test('Rate clicks pin the exact bucket and keep slope in the same card', async ({
  page,
}) => {
  const counter = {
    ...metric,
    metricType: 'Sum',
    isMonotonic: true,
    aggregationTemporalityCode: 2,
    aggregationTemporality: 'Cumulative',
    datapointCount: 4,
    timeseries: [
      {
        ...timeseries[0]!,
        datapoints: timeseries[0]!.datapoints.slice(0, 4).map(point => ({
          ...point,
          metricType: 'Sum',
          isMonotonic: true,
          aggregationTemporalityCode: 2,
          aggregationTemporality: 'Cumulative',
        })),
        datapointCount: 4,
        rateStats: { min: 2, max: 6, avg: 4 },
        views: [0, 1, 2, 3].map(index => ({
          bucketStart: String(BigInt(startMs + index * 1000) * 1_000_000n),
          sampleCount: 1,
          sum: index + 1,
          avg: index + 1,
          rate: index === 0 ? null : index * 2,
          slope: index === 2 ? 2 : null,
          hasReset: false,
        })),
      },
    ],
  }
  await page.route('**/rpc', async route => {
    const request = route.request().postDataJSON() as {
      id: number
      method: string
    }
    const result =
      request.method === 'getMetricView'
        ? counter
        : request.method === 'searchMetricSummaries'
          ? [
              {
                ...summary,
                metricType: 'Sum',
                isMonotonic: true,
                aggregationTemporalityCode: 2,
                aggregationTemporality: 'Cumulative',
                seriesCount: 1,
                seriesCardinality: 1,
                dataPointCount: 4,
              },
            ]
          : undefined
    if (result === undefined) await route.fallback()
    else
      await route.fulfill({ json: { jsonrpc: '2.0', id: request.id, result } })
  })
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.goto(`/metrics/${metric.metricRef}?agg=rate`)
  await expect(page.locator('.metric-time-series-chart')).toBeVisible()
  await expect(page.locator('.chart-sticky__card')).toHaveCount(0)
  await expect(page.locator('.selection-dot--selected')).toHaveCount(0)
  expect(new URL(page.url()).searchParams.has('dp')).toBe(false)
  const graph = page.getByRole('application', {
    name: 'Metric time series chart',
    exact: true,
  })
  await graph.focus()
  await graph.press('Home')
  await graph.press('Enter')
  const sticky = page.locator('.chart-sticky__card')
  await expect(sticky).toBeVisible()
  const firstBucket = await page
    .locator('.selection-dot--selected')
    .evaluate(point => ({
      x: Number(point.getAttribute('cx')),
      y: Number(point.getAttribute('cy')),
    }))
  await graph.press('ArrowRight')
  await graph.press('Enter')
  await expect(sticky.getByText('slope', { exact: true })).toBeVisible()
  const slopeGuide = page.locator('.series-slope-line')
  await expect(slopeGuide).toHaveCSS('stroke-dasharray', '4px, 3px')
  await expect(slopeGuide).toHaveCSS('opacity', '0.35')
  await expect(slopeGuide).toHaveCSS('pointer-events', 'none')
  const slopeDrawing = await page
    .locator('.metric-time-series-chart')
    .evaluate((chart, firstBucket) => {
      const guide = chart.querySelector('.series-slope-line')!
      const point = chart.querySelector('.selection-dot--selected')!
      const seriesColour = getComputedStyle(point).fill
      const series = [...chart.querySelectorAll('path')].find(
        path => getComputedStyle(path).stroke === seriesColour
      )!
      const x1 = Number(guide.getAttribute('x1'))
      const y1 = Number(guide.getAttribute('y1'))
      const x2 = Number(guide.getAttribute('x2'))
      const y2 = Number(guide.getAttribute('y2'))
      const px = Number(point.getAttribute('cx'))
      const py = Number(point.getAttribute('cy'))
      return {
        behindSeries: !!(
          guide.compareDocumentPosition(series) &
          Node.DOCUMENT_POSITION_FOLLOWING
        ),
        extendsBeyondBucket:
          Math.hypot(x2 - x1, y2 - y1) >
          Math.hypot(px - firstBucket.x, py - firstBucket.y) + 1,
        distanceFromPoint:
          Math.abs((px - x1) * (y2 - y1) - (py - y1) * (x2 - x1)) /
          Math.hypot(x2 - x1, y2 - y1),
        colour: getComputedStyle(guide).stroke,
        axisColour: getComputedStyle(
          chart.querySelector('.lc-axis-tick-label')!
        ).fill,
        cardColour: getComputedStyle(
          chart.querySelector(
            '.metric-time-series-chart__selection-legend .chart-selection-legend__row:last-child .chart-selection-legend__dot'
          )!
        ).backgroundColor,
      }
    }, firstBucket)
  expect(slopeDrawing.behindSeries).toBe(true)
  expect(slopeDrawing.extendsBeyondBucket).toBe(true)
  expect(slopeDrawing.distanceFromPoint).toBeLessThan(0.01)
  expect(slopeDrawing.colour).toBe(slopeDrawing.cardColour)
  expect(slopeDrawing.colour).toBe(slopeDrawing.axisColour)
  await expect(
    page.locator('.series-stat-tooltip').filter({ hasText: 'slope' })
  ).toHaveCount(0)
  const selected = page.locator('.selection-dot--selected')
  const box = (await selected.boundingBox())!
  const x = box.x + box.width / 2
  const y = box.y + box.height / 2
  await graph.press('Escape')
  await expect(sticky).toHaveCount(0)
  await page.mouse.move(x, y)
  await expect(page.locator('.lc-tooltip-header').first()).toBeVisible()
  await page.mouse.click(x, y)
  await expect(sticky.getByText('slope', { exact: true })).toBeVisible()
  expect(new URL(page.url()).searchParams.has('dp')).toBe(false)
  await expect(page.locator('.selection-overlay .highlight-rule')).toHaveCount(
    0
  )
  const guideColors = await page
    .locator('.series-slope-line')
    .evaluateAll(lines => lines.map(line => getComputedStyle(line).stroke))
  expect(guideColors.length).toBeGreaterThan(0)
  expect(new Set(guideColors).size).toBe(1)
})

test('raw plot clicks still select source points beneath passive overlays', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.goto(`/metrics/${metric.metricRef}?agg=raw`)
  const graph = page.getByRole('application', {
    name: 'Metric time series chart',
    exact: true,
  })
  await graph.focus()
  await graph.press('Home')
  await graph.press('ArrowRight')
  await graph.press('Enter')
  await expect(page).toHaveURL(/dp=first-/)
  const selectedID = new URL(page.url()).searchParams.get('dp')!
  const selectedMark = page.locator('.selection-dot--selected')
  await expect(selectedMark).toBeVisible()
  const mark = (await selectedMark.boundingBox())!
  const x = mark.x + mark.width / 2
  const y = mark.y + mark.height / 2
  await graph.press('Escape')
  await expect(page.locator('.chart-sticky__card')).toHaveCount(0)
  await page.mouse.move(x, y)
  await expect(page.locator('.lc-tooltip-header').first()).toBeVisible()
  await page.mouse.click(x, y)
  await expect
    .poll(() => new URL(page.url()).searchParams.get('dp'))
    .toBe(selectedID)
})

test('series open by their headers, one at a time, with one combined table', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.goto(`/metrics/${metric.metricRef}?agg=raw`)
  const details = page.getByRole('region', {
    name: 'Metric details',
    exact: true,
  })
  const seriesBubble = details.locator('details.collapse').nth(0)
  const fieldsBubble = details.locator('details.collapse').nth(1)
  const mainHeader = page.getByRole('region', {
    name: 'Metric chart',
    exact: true,
  })
  const darkest = await page.evaluate(() => {
    const probe = document.createElement('span')
    probe.style.backgroundColor = 'var(--color-base-100)'
    document.body.append(probe)
    const color = getComputedStyle(probe).backgroundColor
    probe.remove()
    return color
  })
  await expect(page.locator('.page-layout__main-chrome')).toHaveCSS(
    'background-color',
    darkest
  )
  await expect(page.locator('.page-layout__main-chrome')).toHaveCSS(
    'border-width',
    '0px'
  )
  for (const bar of await page.locator('.metric-chart-control-bar').all())
    await expect(bar).toHaveCSS('background-color', darkest)
  await expect(mainHeader).toHaveCSS('border-radius', '12px')
  const range = mainHeader.locator('[aria-label="Chart time range"]')
  await expect(
    mainHeader.getByRole('checkbox', { name: 'Show chart stat overlays' })
  ).toBeVisible()
  await expect(
    page
      .locator('.metric-chart-view')
      .getByRole('checkbox', { name: 'Show chart stat overlays' })
  ).toHaveCount(0)
  await expect(range).toHaveCount(1)
  await expect(range).toContainText('start:')
  await expect(range).toContainText('end:')
  await expect(
    page.locator('.metric-chart-view [aria-label="Chart time range"]')
  ).toHaveCount(0)
  const headerLayout = await mainHeader.evaluate(header => {
    const top = header
      .querySelector('.pane-header__top')!
      .getBoundingClientRect()
    const badges = header
      .querySelector('.pane-header__badges')!
      .getBoundingClientRect()
    const tabs = header
      .querySelector('[role="tablist"]')!
      .getBoundingClientRect()
    const range = header
      .querySelector('[aria-label="Chart time range"]')!
      .getBoundingClientRect()
    return {
      badgeInset: top.right - badges.right,
      topBottom: top.bottom,
      tabsTop: tabs.top,
      rangeTop: range.top,
    }
  })
  expect(Math.abs(headerLayout.badgeInset)).toBeLessThan(1)
  expect(headerLayout.tabsTop).toBeGreaterThanOrEqual(headerLayout.topBottom)
  expect(headerLayout.rangeTop).toBeGreaterThanOrEqual(headerLayout.topBottom)
  await expect(mainHeader).toHaveCSS(
    'background-color',
    await fieldsBubble
      .locator(':scope > summary')
      .evaluate(element => getComputedStyle(element).backgroundColor)
  )
  const series = page.locator('details[name="metric-series"]')
  await expect(series).toHaveCount(2)
  for (const item of await series.all()) {
    const card = item.locator(':scope > summary')
    await expect(card).toHaveClass(/item-state/)
    await expect(card).toHaveCSS('border-radius', '12px')
    await expect(card).toHaveCSS('padding-top', '10px')
    await expect(card).toHaveCSS('padding-bottom', '10px')
  }
  const firstCheckbox = page.getByRole('checkbox', {
    name: 'Show route: /first in chart',
  })
  await firstCheckbox.uncheck()
  await expect(series.nth(0)).not.toHaveAttribute('open')
  await firstCheckbox.check()
  await expect(series.nth(0)).not.toHaveAttribute('open')
  await expect(seriesBubble).toHaveAttribute('open')
  await expect(fieldsBubble).toHaveAttribute('open')
  await expect(fieldsBubble.locator(':scope > summary')).toHaveText('Metric')
  const badgeInset = await seriesBubble
    .locator(':scope > summary')
    .evaluate(element => {
      const badge = element.querySelector('.badge-count')!
      return (
        element.getBoundingClientRect().right -
        badge.getBoundingClientRect().right -
        parseFloat(getComputedStyle(element).paddingRight) -
        parseFloat(getComputedStyle(element).borderRightWidth)
      )
    })
  expect(Math.abs(badgeInset)).toBeLessThan(1)
  for (const bubble of [seriesBubble, fieldsBubble]) {
    const icon = bubble.locator(':scope > summary svg')
    await expect(icon).toHaveCSS('width', '16px')
    await expect(icon).toHaveCSS('height', '16px')
    await expect(icon).toHaveClass(/text-rp-muted/)
  }
  await expect(
    details.locator('.collapse-arrow, .field-group__caret')
  ).toHaveCount(0)
  await series.nth(0).locator(':scope > summary').click()
  await expect(series.nth(0)).toHaveAttribute('open')
  await expect(series.nth(0).locator('table')).toHaveCount(1)
  const table = series.nth(0).getByRole('table', { name: 'Datapoints' })
  const attribute = table.locator('[data-field="route"]')
  await expect(attribute.locator('.detail-property__key')).toHaveText('route')
  await expect(attribute.locator('.detail-property__type')).toHaveText('string')
  const fieldLayout = await attribute
    .locator('.detail-property__header')
    .evaluate(header => {
      const key = header
        .querySelector('.detail-property__key')!
        .getBoundingClientRect()
      const badges = header
        .querySelector('.detail-property__badges')!
        .getBoundingClientRect()
      const copy = header.querySelector('button')!.getBoundingClientRect()
      return {
        keyRight: key.right,
        badgeLeft: badges.left,
        badgeRight: badges.right,
        copyLeft: copy.left,
        copyInset: header.getBoundingClientRect().right - copy.right,
      }
    })
  expect(fieldLayout.badgeLeft).toBeGreaterThan(fieldLayout.keyRight)
  expect(fieldLayout.copyLeft - fieldLayout.badgeRight).toBeCloseTo(8, 0)
  expect(Math.abs(fieldLayout.copyInset)).toBeLessThan(1)
  await expect(table).toContainText('/first')
  await expect(table.locator('tr[data-dp-id]')).toHaveCount(25)
  const datapoint = table.locator('tr[data-dp-id]').first()
  await datapoint.hover()
  await expect(datapoint.locator('td').first()).toHaveCSS(
    'border-top-left-radius',
    '12px'
  )
  await expect(datapoint.locator('td').last()).toHaveCSS(
    'border-bottom-right-radius',
    '12px'
  )
  const plot = page.locator('.metric-time-series-chart__plot')
  const plotBeforeSelection = (await plot.boundingBox())!
  await datapoint.click()
  await expect(datapoint).toHaveAttribute('aria-selected', 'true')
  const sticky = page.locator('.chart-sticky__card')
  await expect(sticky).toBeVisible()
  await expect(sticky).toHaveCSS('pointer-events', 'none')
  expect(
    await sticky.evaluate(element => {
      const box = element.getBoundingClientRect()
      const hit = document.elementFromPoint(
        box.x + box.width / 2,
        box.y + box.height / 2
      )
      return !!hit && !element.contains(hit)
    })
  ).toBe(true)
  const selectedMark = page.locator('.selection-dot--selected')
  await expect(selectedMark).toBeVisible()
  const plotAfterSelection = (await plot.boundingBox())!
  expect(plotAfterSelection.height).toBeCloseTo(plotBeforeSelection.height, 0)
  const cardBox = (await sticky.boundingBox())!
  const markBox = (await selectedMark.boundingBox())!
  expect(cardBox.x).toBeGreaterThanOrEqual(plotAfterSelection.x)
  expect(cardBox.y).toBeGreaterThanOrEqual(plotAfterSelection.y)
  expect(cardBox.x + cardBox.width).toBeLessThanOrEqual(
    plotAfterSelection.x + plotAfterSelection.width + 1
  )
  expect(cardBox.y + cardBox.height).toBeLessThanOrEqual(
    plotAfterSelection.y + plotAfterSelection.height + 1
  )
  const markX = markBox.x + markBox.width / 2
  const markY = markBox.y + markBox.height / 2
  const distance = Math.hypot(
    Math.max(cardBox.x - markX, 0, markX - cardBox.x - cardBox.width),
    Math.max(cardBox.y - markY, 0, markY - cardBox.y - cardBox.height)
  )
  expect(distance).toBeLessThanOrEqual(24)
  await expect(datapoint.locator('td').first()).toHaveCSS(
    'border-top-left-radius',
    '12px'
  )
  const nextPage = series.nth(0).getByRole('button', {
    name: 'Next page',
    exact: true,
  })
  await expect(nextPage).toBeInViewport({ ratio: 1 })
  await nextPage.click()
  await expect(table.locator('tr[data-dp-id]')).toHaveCount(15)
  await expect(table.locator('tr[data-dp-id]').first()).toHaveAttribute(
    'data-dp-id',
    'first-25'
  )
  await expect(series.nth(0)).toContainText('26-40 of 40 datapoints')
  await expect(nextPage).toBeDisabled()
  await series
    .nth(0)
    .getByRole('button', { name: 'First page', exact: true })
    .click()
  await expect(table.locator('tr[data-dp-id]').first()).toHaveAttribute(
    'data-dp-id',
    'first-0'
  )
  await page.setViewportSize({ width: 1440, height: 720 })
  await expect(nextPage).toBeInViewport({ ratio: 1 })
  await series
    .nth(0)
    .getByRole('combobox', { name: 'Rows per page' })
    .selectOption('50')
  await expect(table.locator('tr[data-dp-id]')).toHaveCount(40)
  await expect(nextPage).toBeInViewport({ ratio: 1 })
  // Both long sections can share the available height when constrained.
  // With enough room, each section must size to its own content instead.
  await page.setViewportSize({ width: 1920, height: 2400 })
  await expect(series.nth(0).locator('details')).toHaveCount(0)
  await expect(series.nth(0)).not.toContainText('service.name')
  const resource = fieldsBubble.locator('[data-field="service.name"]')
  await expect(resource.locator('.detail-property__key')).toHaveText(
    'service.name'
  )
  await expect(resource.locator('.detail-property__origin')).toHaveText(
    'resource'
  )
  await expect
    .poll(async () => (await seriesBubble.boundingBox())!.height)
    .toBeGreaterThan((await fieldsBubble.boundingBox())!.height)
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page
    .getByRole('checkbox', { name: 'Show route: /first in chart' })
    .uncheck()
  await expect(series.nth(0)).toHaveAttribute('open')
  await series.nth(1).locator(':scope > summary').click()
  await expect(series.nth(1)).toHaveAttribute('open')
  await expect(series.nth(0)).not.toHaveAttribute('open')
  await expect(series.locator('table:visible')).toHaveCount(1)
  await expect(
    series.nth(1).getByRole('table', { name: 'Datapoints' })
  ).toContainText('/second')
  await series.nth(0).locator(':scope > summary').focus()
  await page.keyboard.press('Enter')
  await expect(series.nth(0)).toHaveAttribute('open')
  await expect(series.nth(1)).not.toHaveAttribute('open')
})

test('Copy metric stays in its heading and copies exact OTLP only for the selected metric', async ({
  page,
  context,
}) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.setViewportSize({ width: 1920, height: 1000 })
  const raw =
    '{"resourceMetrics":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"checkout-api"}}]},"scopeMetrics":[{"scope":{"name":"instrumentation"},"metrics":[{"name":"request.duration","gauge":{"dataPoints":[{"timeUnixNano":"18446744073709551615","asDouble":-0.0},{"asInt":"9223372036854775807"}]}}]}]}]}\n'
  let exportRequests = 0
  await page.route('**/export/metrics/**', async route => {
    expect(new URL(route.request().url()).pathname).toBe(
      `/export/metrics/${metric.metricRef}`
    )
    exportRequests++
    await route.fulfill(
      exportRequests === 1
        ? { status: 503, contentType: 'text/plain', body: 'export unavailable' }
        : { contentType: 'application/json', body: raw }
    )
  })
  await page.goto(`/metrics/${metric.metricRef}?agg=raw`)
  const detail = page.getByRole('region', {
    name: 'Metric details',
    exact: true,
  })
  const bubble = detail.locator('.detail-bubble__with-action')
  const heading = bubble.locator('summary').first()
  const copy = bubble.getByRole('button', { name: 'Copy metric', exact: true })
  await expect(copy).toBeVisible()
  await expect(copy).toHaveCSS('opacity', '1')
  expect(exportRequests).toBe(0)
  expect(await copy.evaluate(element => element.closest('summary'))).toBeNull()
  const iconBox = (await heading.locator('svg').boundingBox())!
  const copyBox = (await copy.boundingBox())!
  const headingBox = (await heading.boundingBox())!
  expect(
    Math.abs(iconBox.y + iconBox.height / 2 - copyBox.y - copyBox.height / 2)
  ).toBeLessThan(1)
  expect(
    headingBox.x + headingBox.width - copyBox.x - copyBox.width
  ).toBeCloseTo(12, 0)
  await heading.click()
  await expect(bubble.locator('details')).not.toHaveAttribute('open')
  await expect(copy).toBeVisible()
  await page.evaluate(() => navigator.clipboard.writeText('keep my clipboard'))
  await copy.click()
  const status = bubble.locator('.detail-bubble__action').getByRole('status')
  await expect(status).toHaveText('Could not copy metric.')
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    'keep my clipboard'
  )
  await copy.focus()
  await page.keyboard.press('Enter')
  await expect(status).toHaveText('Copied metric.')
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(raw)
  expect(exportRequests).toBe(2)
  await expect(bubble.locator('details')).not.toHaveAttribute('open')
  await expect(page).toHaveURL(`/metrics/${metric.metricRef}?agg=raw`)
})
