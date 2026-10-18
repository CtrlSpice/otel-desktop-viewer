import { expect, test } from '@playwright/test'
import type {
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
        averageLabelAtRight: Math.abs(labelBox.right - lineBox.right) < 1,
      }
    })
  expect(drawing).toEqual({
    extremaMatch: true,
    seriesFound: true,
    averageBehindSeries: true,
    averageHasOwnColour: true,
    averageLabelMatches: true,
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
    datapointCount: 3,
    timeseries: [
      {
        ...timeseries[0]!,
        datapoints: timeseries[0]!.datapoints.slice(0, 3).map(point => ({
          ...point,
          metricType: 'Sum',
          isMonotonic: true,
          aggregationTemporalityCode: 2,
          aggregationTemporality: 'Cumulative',
        })),
        datapointCount: 3,
        rateStats: { min: 2, max: 4, avg: 3 },
        views: [0, 1, 2].map(index => ({
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
                dataPointCount: 3,
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
  await graph.press('ArrowRight')
  await graph.press('Enter')
  await expect(sticky.getByText('slope', { exact: true })).toBeVisible()
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
    .locator('.series-stat-line')
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
        parseFloat(getComputedStyle(element).paddingRight)
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
