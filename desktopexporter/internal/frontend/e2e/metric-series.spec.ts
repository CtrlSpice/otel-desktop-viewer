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

test('series open by their headers, one at a time, with one combined table', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
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
  await page.goto(`/metrics/${metric.metricRef}?agg=raw`)
  const details = page.getByRole('region', {
    name: 'Metric details',
    exact: true,
  })
  const seriesBubble = details.locator('details.collapse').nth(0)
  const fieldsBubble = details.locator('details.collapse').nth(1)
  const series = page.locator('details[name="metric-series"]')
  await expect(series).toHaveCount(2)
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
  await expect(table).toContainText('/first')
  await expect(table.locator('tr[data-dp-id]')).toHaveCount(25)
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
