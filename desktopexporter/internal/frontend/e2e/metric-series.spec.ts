import { routeTelemetry, type FixtureRoute } from './telemetry-fixtures'
import { expect, test, type Page } from '@playwright/test'
import { THEMES, themeRgb } from './theme-colours'
import type { ThemeName } from '../src/themes'
import heatmapPalette from '../src/themes/heatmap-palette.json' with { type: 'json' }
import {
  GRAPH_INKS,
  GRAPH_PALETTE_VERSION,
  GRAPH_SWATCHES,
  graphSwatch,
} from '../src/themes/graph-inks'
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

function rgb(hex: string) {
  return `rgb(${[1, 3, 5].map(i => parseInt(hex.slice(i, i + 2), 16)).join(', ')})`
}

function accentRgb(theme: ThemeName) {
  return themeRgb(theme, 'primary')
}

const countRamps: Record<number, { day: string[]; night: string[] }> =
  heatmapPalette.ramps
function countRamp(theme: ThemeName, steps: number) {
  return countRamps[steps]![theme.endsWith('-hiru') ? 'day' : 'night']
}

// Chromium serializes OKLab mixes differently from hex colours. Resolve the
// exact approved ramp through CSS, including the neutral Text interaction mix.
async function mixedInkColours(page: Page, colours: string[], weight = 100) {
  return page.evaluate(
    ({ colours, weight }) => {
      const probe = document.createElement('span')
      document.body.append(probe)
      const resolved = colours.map(colour => {
        probe.style.color = `color-mix(in oklab, ${colour} ${weight}%, var(--color-base-content))`
        return getComputedStyle(probe).color
      })
      probe.remove()
      return resolved
    },
    { colours, weight }
  )
}

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
  await routeTelemetry(page, async route => {
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

test('metric replacement keeps its displayed identity and mounted headers during delayed loading', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1100 })
  await page.addInitScript(() =>
    localStorage.setItem('signal-detail:open', 'true')
  )
  const replacement = {
    ...metric,
    metricRef: 'replacement',
    name: 'replacement.metric',
  }
  let release: (() => Promise<void>) | undefined
  await routeTelemetry(page, async route => {
    const { id, method, params } = route.request().postDataJSON()
    if (method === 'searchMetricSummaries') {
      await route.fulfill({
        json: {
          jsonrpc: '2.0',
          id,
          result: [
            summary,
            {
              ...summary,
              metricRef: replacement.metricRef,
              name: replacement.name,
            },
          ],
        },
      })
    } else if (
      method === 'getMetricView' &&
      params.metricRef === replacement.metricRef
    ) {
      release = () =>
        route.fulfill({ json: { jsonrpc: '2.0', id, result: replacement } })
    } else await route.fallback()
  })
  await page.goto(`/metrics/${metric.metricRef}?time=all`)
  const fields = page.getByRole('table', { name: 'Metric fields' })
  const header = page.locator('.page-layout__main-chrome > .pane-header')
  await expect(fields).toContainText(metric.name)
  const nodes = await page.evaluateHandle(() => [
    document.querySelector('.page-layout__main-chrome > .pane-header'),
    document.querySelector('[aria-label="Metric fields"]'),
    document.querySelector('#metric-chart-tabpanel'),
  ])
  await page.clock.install()
  await page.clock.pauseAt(new Date())
  await page
    .locator('#signal-drawer-panel')
    .getByRole('button', { name: /replacement.metric/ })
    .click()
  await expect.poll(() => !!release).toBe(true)
  await expect(header).toContainText(metric.name)
  await expect(fields).toContainText(metric.name)
  await expect(page.getByText('Loading selected metric…')).toHaveCount(0)
  await page.clock.runFor(200)
  await expect(page.getByText('Loading selected metric…')).toBeVisible()
  expect(
    await nodes.evaluate(original => original.every(node => node?.isConnected))
  ).toBe(true)
  await release!()
  await expect(header).toContainText(replacement.name)
  await expect(fields).toContainText(replacement.name)
  await expect(page.getByText('Loading selected metric…')).toHaveCount(0)
  expect(
    await nodes.evaluate(original => original.every(node => node?.isConnected))
  ).toBe(true)
})

test('series IDs stay on one line beside a separate compact colour column', async ({
  page,
}) => {
  const ids = [
    '888d17a7-2d32-4945-bc9c-2f4bd2aab16f',
    '47801e24-eb28-4964-b691-79ca4638f84e',
  ]
  await routeTelemetry(page, async route => {
    const request = route.request().postDataJSON()
    if (request.method !== 'getMetricView') return route.fallback()
    await route.fulfill({
      json: {
        jsonrpc: '2.0',
        id: request.id,
        result: {
          ...metric,
          timeseries: timeseries.map((series, index) => ({
            ...series,
            attributes: [
              {
                key: 'session.id',
                value: { kind: 'string', value: ids[index] },
              },
            ],
          })),
        },
      },
    })
  })
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.goto(`/metrics/${metric.metricRef}?agg=raw`)
  const row = page.locator('.ts-row[data-series-key]').first()
  const identifier = row.locator('.ts-row__attrs .identifier')
  await expect(identifier).toHaveAttribute('title', ids[0]!)
  await expect(identifier.locator('[aria-hidden="true"]')).toHaveText(
    '888d17a7…d2aab16f'
  )
  await expect(identifier).toHaveCSS('white-space', 'nowrap')
  const spacing = await row.evaluate(row => {
    const picker = row.querySelector('.series-colour-edit')!
    const dot = picker.querySelector('.rounded-full')!
    const identifier = row.querySelector('.identifier')!
    return {
      separate: picker.closest('td') !== identifier.closest('td'),
      gap:
        identifier.getBoundingClientRect().left -
        dot.getBoundingClientRect().right,
    }
  })
  expect(spacing.separate).toBe(true)
  expect(spacing.gap).toBeGreaterThanOrEqual(8)
  expect(spacing.gap).toBeLessThanOrEqual(16)
  await row
    .getByRole('button', { name: `Select session.id: ${ids[0]}` })
    .click()
  await expect(
    page
      .getByRole('region', { name: 'Selected series datapoints' })
      .locator('tr[data-dp-id]')
  ).toHaveCount(25)
  await row
    .getByRole('button', {
      name: `Expand attributes for session.id: ${ids[0]}`,
    })
    .click()
  await expect(
    page.locator(
      '[data-series-attributes="first"] [data-field="session.id"] .detail-property__body'
    )
  ).toHaveText(ids[0]!)
})

async function expectRoundedAxes(page: Page, colour: string) {
  const axes = page.locator('.metric-axes')
  await expect(axes).toHaveCount(1)
  await expect(axes).toHaveCSS('border-left-width', '1px')
  await expect(axes).toHaveCSS('border-bottom-width', '1px')
  await expect(axes).toHaveCSS('border-left-color', colour)
  await expect(axes).toHaveCSS('border-bottom-left-radius', '12px')
  await expect(axes).toHaveCSS('pointer-events', 'none')
  await expect(axes).toHaveAttribute('aria-hidden', 'true')
  await expect(page.locator('.metric-chart-view .lc-axis-rule')).toHaveCount(0)
  await expect(page.locator('.metric-dot-grid')).toHaveCount(0)
  await expect(page.locator('.metric-chart-view .lc-grid')).toHaveCount(0)
}

async function expectMeasuredAxisGutter(page: Page) {
  const chart = page
    .locator('#metric-chart-tabpanel svg.lc-layout-svg')
    .filter({ has: page.locator('.placement-left') })
  await expect(chart).toBeVisible()
  await page.evaluate(() => document.fonts.ready)
  await expect
    .poll(() =>
      chart.evaluate(svg => {
        const bounds = svg.getBoundingClientRect()
        const root = svg.closest('.metric-chart-plot, .heatmap-wrapper')!
        const axis = root.querySelector('.metric-axes')!.getBoundingClientRect()
        const labels = [
          ...svg.querySelectorAll('.placement-left .lc-axis-tick-label'),
        ]
        const xLabels = [
          ...svg.querySelectorAll('.placement-bottom .lc-axis-tick-label'),
        ]
        const unit = svg
          .querySelector('.placement-left .lc-axis-label')!
          .getBoundingClientRect()
        return {
          labelsPresent: labels.length > 0,
          aligned: labels.every(
            label =>
              Math.abs(axis.left - label.getBoundingClientRect().right - 7.5) <
              1
          ),
          unitAligned: Math.abs(unit.left - axis.left) < 1,
          unitAbove: unit.bottom < axis.top && unit.top >= bounds.top - 1,
          valuesFit: labels.every(
            label => label.getBoundingClientRect().left >= bounds.left - 1
          ),
          xFit: xLabels.every(
            label => label.getBoundingClientRect().left >= bounds.left - 1
          ),
        }
      })
    )
    .toEqual({
      labelsPresent: true,
      aligned: true,
      unitAligned: true,
      unitAbove: true,
      valuesFit: true,
      xFit: true,
    })
}

test('time-series Y-axis labels sit beside the axis and remeasure after resize and larger text', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.goto(`/metrics/${metric.metricRef}?time=all&agg=raw`)
  const header = page.getByRole('region', { name: 'Metric chart', exact: true })
  await expect(header.locator('.pane-header__subtitle')).toHaveCount(0)
  await expect(header.getByRole('img', { name: 'checkout-api' })).toBeVisible()
  await expectMeasuredAxisGutter(page)
  for (const width of [1200, 900, 1920]) {
    await page.setViewportSize({ width, height: 1000 })
    await expectMeasuredAxisGutter(page)
  }
  const axes = page.locator('.metric-axes')
  const before = await axes.evaluate(element =>
    parseFloat((element as HTMLElement).style.left)
  )
  await page
    .locator('html')
    .evaluate(element => (element.style.fontSize = '24px'))
  await expectMeasuredAxisGutter(page)
  await expect
    .poll(() =>
      axes.evaluate(element => parseFloat((element as HTMLElement).style.left))
    )
    .toBeGreaterThan(before)
  await expect(
    page.locator('.placement-left .lc-axis-tick-label').first()
  ).toHaveCSS('font-size', '21px')
})

test('metric descriptions stay in Details rather than summary cards', async ({
  page,
}) => {
  const description =
    'Synthetic heatmap playground, same samples as the explicit histogram: Load burst'
  await routeTelemetry(page, async route => {
    const { id, method } = route.request().postDataJSON()
    if (method === 'searchMetricSummaries')
      return route.fulfill({
        json: { jsonrpc: '2.0', id, result: [{ ...summary, description }] },
      })
    if (method === 'getMetricView')
      return route.fulfill({
        json: { jsonrpc: '2.0', id, result: { ...metric, description } },
      })
    return route.fallback()
  })
  await page.setViewportSize({ width: 1920, height: 1080 })
  await page.goto(`/metrics/${metric.metricRef}`)
  const card = page.getByRole('button', { name: /^request\.duration/ })
  await expect(card).toBeVisible()
  await expect(card).not.toContainText(description)
  await expect(
    card.locator('.signal-row__lead, .signal-row__description')
  ).toHaveCount(0)
  await expect(card.locator('.signal-row__meta-row')).toHaveCount(0)
  const date = card.locator('.signal-row__date')
  await expect(date).toHaveText(/Last seen:.*Last value: 40 ms/)
  await page.evaluate(() => document.fonts.ready)
  const layout = await date.evaluate(element => {
    const [seen, value] = [...element.children].map(child =>
      child.getBoundingClientRect()
    )
    return {
      seen: seen!.toJSON(),
      value: value!.toJSON(),
      overflow: element.scrollWidth > element.clientWidth,
    }
  })
  expect(layout.seen.y).toBeCloseTo(layout.value.y, 0)
  expect(layout.value.x - layout.seen.right).toBeGreaterThanOrEqual(16)
  expect(layout.overflow).toBe(false)
  await expect(
    page
      .getByRole('table', { name: 'Metric fields' })
      .getByText(description, { exact: true })
  ).toBeVisible()
})

test('series header hover and keyboard focus emphasize only the corresponding visible line', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.goto(`/metrics/${metric.metricRef}?agg=raw`)
  const headers = page.locator('.ts-row[data-series-key]')
  const first = page.locator('.metric-series-line[data-series-key="first"]')
  const second = page.locator('.metric-series-line[data-series-key="second"]')
  await expect(first).toBeVisible()
  const url = page.url()
  const path = await first.getAttribute('d')
  await headers.first().hover()
  await expect(first).toHaveAttribute('data-emphasized', 'true')
  await expect(second).toHaveAttribute('data-emphasized', 'false')
  await expect(headers.first().locator('..')).not.toHaveAttribute('open')
  await page.getByRole('region', { name: 'Metric chart', exact: true }).hover()
  await expect(first).toHaveAttribute('data-emphasized', 'false')
  await headers.nth(1).focus()
  await expect(second).toHaveAttribute('data-emphasized', 'true')
  await expect(first).toHaveAttribute('data-emphasized', 'false')
  await headers.nth(1).getByRole('checkbox').focus()
  await expect(second).toHaveAttribute('data-emphasized', 'true')
  await page
    .getByRole('button', { name: 'Download metric', exact: true })
    .focus()
  await expect(second).toHaveAttribute('data-emphasized', 'false')
  await expect(first).toHaveAttribute('d', path!)
  expect(page.url()).toBe(url)
  await headers.first().getByRole('checkbox').uncheck()
  await expect(first).toHaveCount(0)
  await headers.first().hover()
  await expect(second).toHaveAttribute('data-emphasized', 'false')
})

test('time-series axes use the shared rounded Muted border', async ({
  page,
}) => {
  await page.addInitScript(() =>
    localStorage.setItem('theme', 'madoromi-fuyu-yoru')
  )
  await page.goto(`/metrics/${metric.metricRef}`)
  await expect(page.locator('.metric-time-series-chart')).toBeVisible()
  await expectRoundedAxes(page, themeRgb('madoromi-fuyu-yoru', 'muted'))
})

for (const { id: theme } of THEMES) {
  test(`categorical series keep their saved colours across season changes and reloads in ${theme}`, async ({
    page,
  }) => {
    await page.addInitScript(
      ({ theme, version }) => {
        localStorage.setItem('theme', theme)
        const key = `graph-colour-slots:${version}:224:metric:series-layout`
        if (!localStorage.getItem(key))
          localStorage.setItem(
            key,
            JSON.stringify([
              ['first', 0],
              ['second', 1],
            ])
          )
      },
      { theme, version: GRAPH_PALETTE_VERSION }
    )
    await page.setViewportSize({ width: 1920, height: 1000 })
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await page.goto(`/metrics/${metric.metricRef}?agg=raw`)
    const lines = page.locator('.metric-series-line')
    await expect(lines).toHaveCount(2)
    await expect(lines.first()).toBeVisible()
    const drawing = () =>
      lines.evaluateAll(elements =>
        elements.map(element => ({
          key: element.getAttribute('data-series-key'),
          path: element.getAttribute('d'),
          colour: getComputedStyle(element).stroke,
        }))
      )
    const original = await drawing()
    const official = GRAPH_INKS.flatMap(ink => ink.swatches)
    expect(official).toHaveLength(224)
    expect(original.map(line => line.colour)).toEqual(
      [0, 1].map(slot => rgb(GRAPH_SWATCHES[slot]!.color))
    )
    expect(new Set(original.map(line => line.colour)).size).toBe(2)

    await expect(
      page.getByRole('button', { name: /^Colour for route:/ })
    ).toHaveCount(2)
    const second = page.getByRole('checkbox', {
      name: 'Show route: /second in chart',
      exact: true,
    })
    await second.uncheck()
    await expect(lines).toHaveCount(1)
    await second.check()
    await expect(lines).toHaveCount(2)

    await page.getByRole('button', { name: /^Season:/ }).click()
    await page.getByRole('radio', { name: 'Winter' }).click()
    await page.keyboard.press('Escape')
    await expect.poll(drawing).toEqual(original)
    await page.reload()
    await expect(lines).toHaveCount(2)
    await page.locator('.theme-toggle').click()
    await expect.poll(drawing).toEqual(original)
  })
}

test('series colour picker overrides persist independently of visibility and disclosure and reset to automatic', async ({
  page,
}) => {
  await routeTelemetry(page, async route => {
    const request = route.request().postDataJSON()
    if (request.method !== 'getMetricView') return route.fallback()
    await route.fulfill({
      json: {
        jsonrpc: '2.0',
        id: request.id,
        result: {
          ...metric,
          timeseries: timeseries.map(series => ({
            ...series,
            attributes: [
              ...series.attributes,
              {
                key: 'session.id',
                value: {
                  kind: 'string',
                  value: `long-session-identifier-${series.seriesRef}`,
                },
              },
              { key: 'common', value: { kind: 'string', value: 'same' } },
            ],
          })),
        },
      },
    })
  })
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.goto(`/metrics/${metric.metricRef}?agg=raw`)
  const first = page.locator('.metric-series-line[data-series-key="first"]')
  const second = page.locator('.metric-series-line[data-series-key="second"]')
  await expect(first).toBeVisible()
  const original = await first.evaluate(
    element => getComputedStyle(element).stroke
  )
  const other = await second.evaluate(
    element => getComputedStyle(element).stroke
  )
  const picker = page.getByLabel('Colour for route: /first', { exact: true })
  const hex = page.getByRole('textbox', {
    name: 'Colour value',
    exact: true,
  })
  const disclosure = page.locator('.ts-row[data-series-key]').first()
  await expect(disclosure.locator('.ts-row__attrs')).toHaveText('/first')
  await expect(disclosure.locator('.ts-row__attrs')).toHaveAttribute(
    'title',
    /session.id: long-session-identifier-first/
  )
  const attributeDisclosure = disclosure.locator('.ts-row__caret')
  const wasOpen = await attributeDisclosure.evaluate(element =>
    element.getAttribute('aria-expanded')
  )
  const title = (await disclosure.locator('.ts-row__attrs').boundingBox())!
  const edit = (await picker.boundingBox())!
  expect(edit.x + edit.width).toBeLessThanOrEqual(title.x)
  const dot = disclosure.locator('.series-colour-edit > span')
  await page.mouse.move(0, 0)
  await expect(dot).toBeVisible()
  await picker.focus()
  await picker.click()
  await expect(hex).toBeFocused()
  const hue = page.getByRole('slider', { name: 'Hue', exact: true })
  const before = await hue.getAttribute('aria-valuenow')
  await hue.focus()
  await hue.press('ArrowRight')
  await expect(hue).not.toHaveAttribute('aria-valuenow', before!)
  await expect(first).not.toHaveCSS('stroke', original)
  await page.keyboard.press('Escape')
  await expect(picker).toBeFocused()
  expect(await attributeDisclosure.getAttribute('aria-expanded')).toBe(wasOpen)
  await expect(disclosure).toHaveAttribute('aria-selected', 'false')
  const visible = page.getByRole('checkbox', {
    name: 'Show route: /first in chart',
    exact: true,
  })
  await picker.click()
  await hex.fill('#123abc')
  await expect(first).toHaveCSS('stroke', rgb('#123abc'))
  await expect(second).toHaveCSS('stroke', other)
  await expect(dot).toHaveCSS('background-color', rgb('#123abc'))
  await expect(visible).toHaveCSS(
    'color',
    await page
      .getByRole('checkbox', {
        name: 'Show route: /second in chart',
        exact: true,
      })
      .evaluate(element => getComputedStyle(element).color)
  )
  await expect(visible).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
  await expect(visible).toBeChecked()
  expect(await attributeDisclosure.getAttribute('aria-expanded')).toBe(wasOpen)
  await page.keyboard.press('Escape')
  await visible.uncheck()
  await expect(picker).toBeVisible()
  await expect(visible).not.toBeChecked()
  await expect(disclosure.locator('.ts-row__attrs')).toHaveText('/first')
  await picker.click()
  await hex.fill('#654321')
  await expect(dot).toHaveCSS('background-color', rgb('#654321'))
  await expect(first).toHaveCount(0)
  await expect(visible).not.toBeChecked()
  await page
    .getByRole('button', {
      name: 'Reset colour for route: /first to automatic',
      exact: true,
    })
    .click()
  await expect(dot).toHaveCSS('background-color', original)
  await hex.fill('#123abc')
  await page.keyboard.press('Escape')
  await visible.check()
  await expect(first).toHaveCSS('stroke', rgb('#123abc'))
  await picker.click()
  await hex.fill('#a234bc')
  await expect(first).toHaveCSS('stroke', rgb('#a234bc'))
  await page.reload()
  await picker.click()
  await expect(hex).toHaveValue('#a234bc')
  await expect(first).toHaveCSS('stroke', rgb('#a234bc'))
  await disclosure.hover()
  await page
    .getByRole('button', {
      name: 'Reset colour for route: /first to automatic',
      exact: true,
    })
    .click()
  await expect(first).toHaveCSS('stroke', original)
  await expect(second).toHaveCSS('stroke', other)
  await page.reload()
  await expect(first).toHaveCSS('stroke', original)
})

test('series table sorts attributes while retaining checked choices and colours', async ({
  page,
}) => {
  const samples = [
    { key: 'a', model: 'haiku', type: 'input' },
    { key: 'b', model: 'haiku', type: 'output' },
    { key: 'c', model: 'sonnet', type: 'input' },
  ]
  const received: JsonMetricViewData = {
    ...metric,
    timeseries: samples.map(sample => ({
      ...timeseries[0]!,
      seriesRef: sample.key,
      attributes: [
        { key: 'model', value: { kind: 'string', value: sample.model } },
        { key: 'type', value: { kind: 'string', value: sample.type } },
      ],
      datapoints: timeseries[0]!.datapoints.map((point, index) => ({
        ...point,
        id: `${sample.key}-${index}`,
      })),
    })),
  }
  let requested: string[] = []
  await routeTelemetry(page, async route => {
    const request = route.request().postDataJSON()
    if (request.method === 'getMetricView')
      return route.fulfill({
        json: { jsonrpc: '2.0', id: request.id, result: received },
      })
    if (request.method === 'getMetricAggregateView') {
      requested = request.params.seriesRefs
      expect(request.params.selectedSeriesRefs).toEqual([])
    }
    return route.fallback()
  })
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.goto(`/metrics/${metric.metricRef}?agg=raw`)
  const grid = page.getByRole('grid', { name: 'Metric series', exact: true })
  const rows = grid.locator('.ts-row:visible')
  const lines = page.locator('.metric-series-line')
  await expect(lines).toHaveCount(3)
  const colors = await lines.evaluateAll(elements =>
    Object.fromEntries(
      elements.map(element => [
        element.getAttribute('data-series-key'),
        getComputedStyle(element).stroke,
      ])
    )
  )
  await grid.getByRole('button', { name: 'Sort by model', exact: true }).click()
  await grid.getByRole('button', { name: 'Sort by model', exact: true }).click()
  await expect(rows.first()).toHaveAttribute('data-series-key', 'c')
  await expect(
    grid.getByRole('columnheader').filter({
      has: page.getByRole('button', { name: 'Sort by model', exact: true }),
    })
  ).toHaveAttribute('aria-sort', 'descending')
  await expect(grid.getByRole('button', { name: /^Filter / })).toHaveCount(0)
  await expect(rows).toHaveCount(3)
  await expect(lines).toHaveCount(3)
  await grid
    .getByRole('checkbox', {
      name: 'Show model: haiku · type: input in chart',
      exact: true,
    })
    .uncheck()
  await grid.getByRole('button', { name: 'Sort by type', exact: true }).click()
  await grid.getByRole('button', { name: 'Sort by type', exact: true }).click()
  await expect(rows.first()).toHaveAttribute('data-series-key', 'b')
  await expect(rows).toHaveCount(3)
  await expect(lines).toHaveCount(2)
  expect(requested).toEqual([])
  await expect(
    grid.getByRole('checkbox', {
      name: 'Show model: haiku · type: input in chart',
      exact: true,
    })
  ).not.toBeChecked()
  for (const key of ['b', 'c'])
    await expect(
      page.locator(`.metric-series-line[data-series-key="${key}"]`)
    ).toHaveCSS('stroke', colors[key]!)
  await expect(
    grid.getByRole('button', { name: 'Sort by model', exact: true })
  ).toBeVisible()
  await expect(
    grid.getByRole('button', { name: 'Sort by type', exact: true })
  ).toBeVisible()
  await page
    .getByRole('checkbox', { name: 'Show combined average', exact: true })
    .check()
  await expect.poll(() => requested).toEqual(['b', 'c'])
  await expect(
    page.getByRole('checkbox', { name: /across all series/ })
  ).toHaveCount(0)
  await page.setViewportSize({ width: 390, height: 844 })
  await page
    .getByRole('button', { name: 'Collapse sidebar', exact: true })
    .click()
  await expect(
    page.getByRole('region', {
      name: 'Metric series table viewport',
      exact: true,
    })
  ).toBeVisible()
  expect(
    await grid.evaluate(element => element.getBoundingClientRect().width)
  ).toBeGreaterThan(300)
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

  async function holdAggregates(
    page: Page,
    includeNextMetric = false,
    cumulativeReadings?: number
  ) {
    const pending: { route: FixtureRoute; wholeWindow: boolean }[] = []
    const temporality =
      cumulativeReadings === undefined
        ? { aggregationTemporalityCode: 1, aggregationTemporality: 'Delta' }
        : {
            aggregationTemporalityCode: 2,
            aggregationTemporality: 'Cumulative',
          }
    const histogramMetric: JsonMetricViewData = {
      ...metric,
      metricType: 'Histogram',
      ...temporality,
      timeseries: timeseries.map(series => ({
        ...series,
        datapointCount: cumulativeReadings ?? series.datapointCount,
        datapoints: series.datapoints
          .slice(0, cumulativeReadings ?? 1)
          .map(datapoint => ({
            ...datapoint,
            metricType: 'Histogram',
            ...temporality,
            count: '12',
            sum: 18,
            min: 0,
            max: 4,
            explicitBounds: [1, 2],
            bucketCounts: ['2', '7', '3'],
            quantiles: bucket.quantiles,
          })),
      })),
    }
    const histogramSummary: JsonMetricSummary = {
      ...summary,
      metricType: 'Histogram',
      ...temporality,
    }
    await routeTelemetry(page, async route => {
      const request = route.request().postDataJSON()
      if (request.method === 'getMetricAggregateView') {
        pending.push({ route, wholeWindow: request.params.targetBuckets === 1 })
        return
      }
      const result =
        request.method === 'getMetricView'
          ? {
              ...histogramMetric,
              metricRef: request.params.metricRef,
              timeseries: histogramMetric.timeseries
                .filter(
                  series =>
                    !request.params.seriesRefs ||
                    request.params.seriesRefs.includes(series.seriesRef)
                )
                .map(series =>
                  cumulativeReadings === undefined
                    ? series
                    : {
                        ...series,
                        datapoints:
                          request.params.targetBuckets === 0
                            ? series.datapoints.map(point => ({
                                ...point,
                                quantiles: undefined,
                              }))
                            : series.datapoints.slice(1).map(point => ({
                                ...point,
                                count: '0',
                                sum: 0,
                                bucketCounts: ['0', '0', '0'],
                                quantiles: null,
                              })),
                      }
                ),
            }
          : request.method === 'searchMetricSummaries'
            ? [
                histogramSummary,
                ...(includeNextMetric
                  ? [
                      {
                        ...histogramSummary,
                        metricRef: `${metric.metricRef}-next`,
                        name: 'next.histogram',
                        lastSeen: String(BigInt(endNs) - 1n),
                      },
                    ]
                  : []),
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

  async function respond(route: FixtureRoute, value = bucket, fail = false) {
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

  test('retains histogram aggregates through a pending and failed metric replacement', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 1000 })
    const pending = await holdAggregates(page, true)
    await Promise.all(pending.map(({ route }) => respond(route)))
    const bars = page.locator('.heatmap-cell')
    await expect(bars).toHaveCount(3)
    const before = await bars.evaluateAll(elements =>
      elements.map(element => element.getAttribute('fill'))
    )
    let replacement: FixtureRoute | undefined
    await routeTelemetry(page, route => {
      const request = route.request().postDataJSON()
      if (
        request.method === 'getMetricView' &&
        request.params.metricRef === `${metric.metricRef}-next`
      ) {
        replacement = route
        return
      }
      return route.fallback()
    })
    await page.getByRole('button', { name: /next.histogram/ }).click()
    await expect.poll(() => Boolean(replacement)).toBe(true)
    await expect(bars).toHaveCount(3)
    expect(
      await bars.evaluateAll(elements =>
        elements.map(element => element.getAttribute('fill'))
      )
    ).toEqual(before)
    await replacement!.fulfill({
      json: {
        jsonrpc: '2.0',
        id: replacement!.request().postDataJSON().id,
        error: { code: -32603, message: 'replacement unavailable' },
      },
    })
    await expect(
      page.getByText('Could not load the selected metric.')
    ).toBeVisible()
    await expect(bars).toHaveCount(3)
    expect(
      await bars.evaluateAll(elements =>
        elements.map(element => element.getAttribute('fill'))
      )
    ).toEqual(before)
  })

  test('histogram and heatmap gutters fit long counts and high-precision bucket labels', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 1000 })
    const pending = await holdAggregates(page)
    const large: JsonAggregateBucket = {
      ...bucket,
      count: 9_000_000_000_000_000,
      sum: 1e20,
      min: 0,
      max: 246_913,
      quantiles: null,
      explicitBounds: [123_456.7890123451, 123_456.7890123452],
      bucketCounts: [
        3_000_000_000_000_000, 3_000_000_000_000_000, 3_000_000_000_000_000,
      ],
    }
    for (const { route } of pending) await respond(route, large)
    for (const tab of ['Heatmap', 'Histogram']) {
      await page.getByRole('tab', { name: tab, exact: true }).click()
      await expectMeasuredAxisGutter(page)
      await page.setViewportSize({ width: 900, height: 1000 })
      await expectMeasuredAxisGutter(page)
      await page.setViewportSize({ width: 1440, height: 1000 })
    }
  })

  test('one count ramp retains exact colours and geometry across seasons and reloads', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1920, height: 1000 })
    await page.emulateMedia({ colorScheme: 'light', reducedMotion: 'reduce' })
    const pending = await holdAggregates(page)
    for (const { route } of pending) await respond(route)
    // Fulfil subsequent navigation/reload requests with the same distribution.
    await routeTelemetry(page, route =>
      route.request().postDataJSON().method === 'getMetricAggregateView'
        ? respond(route)
        : route.fallback()
    )
    await expect(page.locator('html')).toHaveAttribute(
      'data-theme',
      'madoromi-aki-hiru'
    )
    const geometry = () =>
      page
        .locator('.heatmap-cell')
        .evaluateAll(cells =>
          cells.map(cell =>
            ['x', 'y', 'width', 'height'].map(attr => cell.getAttribute(attr))
          )
        )
    await expect(page.locator('.heatmap-cell')).toHaveCount(3)
    await expect(page.locator('.heatmap-cell').first()).toBeVisible()
    const original = await geometry()
    const assertCountInk = async () => {
      // Closing a picker can leave the pointer over a cell's hover highlight.
      await page.mouse.move(0, 0)
      const ramp = countRamp('madoromi-aki-hiru', 4)
      await expect
        .poll(() =>
          page
            .locator('.heatmap-legend__swatch')
            .evaluateAll(dots =>
              dots.map(dot => getComputedStyle(dot).backgroundColor)
            )
        )
        .toEqual(ramp.map(rgb))
      const fills = await mixedInkColours(page, ramp)
      await expect
        .poll(() =>
          page
            .locator('.heatmap-cell')
            .evaluateAll(cells =>
              cells.map(cell => getComputedStyle(cell).fill).sort()
            )
        )
        .toEqual([fills[0], fills[1], fills[3]].sort())
      expect(await geometry()).toEqual(original)
    }
    await assertCountInk()
    await expect(
      page.getByRole('button', { name: 'Histogram count palette', exact: true })
    ).toHaveCount(0)
    await page.locator('.theme-toggle').click()
    await expect(page.locator('html')).toHaveAttribute(
      'data-theme',
      'madoromi-aki-yoru'
    )
    await expect
      .poll(() =>
        page
          .locator('.heatmap-legend__swatch')
          .evaluateAll(dots =>
            dots.map(dot => getComputedStyle(dot).backgroundColor)
          )
      )
      .toEqual(countRamp('madoromi-aki-yoru', 4).map(rgb))
    await page.locator('.theme-toggle').click()
    await expect(page.locator('html')).toHaveAttribute(
      'data-theme',
      'madoromi-aki-hiru'
    )
    const themeTrigger = page.getByRole('button', { name: /^Season:/ })
    await themeTrigger.click()
    const themePicker = page.locator('.theme-picker-popover:popover-open')
    await themePicker.getByRole('radio', { name: 'Winter' }).click()
    await assertCountInk()
    await expect(themePicker).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(themeTrigger).toBeFocused()
    await expect(themePicker).not.toBeVisible()
    await page.reload()
    await assertCountInk()
    await page.getByRole('tab', { name: 'Histogram', exact: true }).click()
    await expect
      .poll(() =>
        page
          .locator('.lc-bars-bar')
          .evaluateAll(bars => bars.map(bar => getComputedStyle(bar).fill))
      )
      .toEqual(
        [0, 3, 1].map(index => rgb(countRamp('madoromi-aki-hiru', 4)[index]!))
      )
  })

  for (let steps = 1; steps <= 12; steps++) {
    test(`count ink adapts to ${steps} distinct counts without changing bucket order`, async ({
      page,
    }) => {
      await page.addInitScript(() =>
        localStorage.setItem('theme', 'madoromi-aki-hiru')
      )
      const pending = await holdAggregates(page)
      for (const { route } of pending)
        await respond(route, {
          ...bucket,
          count: (steps * (steps + 1)) / 2,
          explicitBounds: Array.from(
            { length: steps - 1 },
            (_, index) => index + 1
          ),
          bucketCounts: Array.from({ length: steps }, (_, index) => index + 1),
        })
      for (const tab of ['Heatmap', 'Histogram']) {
        await page.getByRole('tab', { name: tab, exact: true }).click()
        const legend = page.getByRole('list', {
          name: `${tab} count scale`,
          exact: true,
        })
        await expect(legend.getByRole('listitem')).toHaveText(
          Array.from({ length: steps }, (_, index) => String(index + 1))
        )
        expect(
          await legend
            .locator('.heatmap-legend__swatch')
            .evaluateAll(dots =>
              dots.map(dot => getComputedStyle(dot).backgroundColor)
            )
        ).toEqual(countRamp('madoromi-aki-hiru', steps).map(rgb))
        const marks = page.locator(
          tab === 'Heatmap' ? '.heatmap-cell' : '.lc-bars-bar'
        )
        await expect(marks).toHaveCount(steps)
        const expected =
          tab === 'Heatmap'
            ? await mixedInkColours(page, countRamp('madoromi-aki-hiru', steps))
            : countRamp('madoromi-aki-hiru', steps).map(rgb)
        expect(
          await marks.evaluateAll(marks =>
            marks.map(mark => getComputedStyle(mark).fill)
          )
        ).toEqual(expected)
      }
    })
  }

  for (const [readings, message] of [
    [
      2,
      'No interval observations in this range. Cumulative charts show changes between readings; recorded totals are available in Details.',
    ],
    [
      1,
      'Not enough cumulative readings to calculate interval activity. Recorded totals are available in Details.',
    ],
    [0, 'No bucket data in range'],
  ] as const) {
    test(`cumulative histogram empty state distinguishes ${readings} received readings`, async ({
      page,
    }) => {
      const pending = await holdAggregates(page, false, readings)
      // A received empty result must not restart detail/aggregate fetches each
      // time the 120ms newly-visible-series debounce expires.
      await page.waitForTimeout(400)
      expect(pending).toHaveLength(2)
      for (const { route } of pending) {
        await route.fulfill({
          json: {
            jsonrpc: '2.0',
            id: route.request().postDataJSON().id,
            result: {
              aggregate:
                readings > 1
                  ? [
                      {
                        ...bucket,
                        count: 0,
                        sum: 0,
                        bucketCounts: [0, 0, 0],
                        quantiles: null,
                        min: undefined,
                        max: undefined,
                      },
                    ]
                  : [],
              scalarAggregate: null,
            },
          },
        })
      }
      for (const tab of ['Heatmap', 'Histogram']) {
        const button = page.getByRole('tab', { name: tab, exact: true })
        await button.click()
        await expect(button).toHaveAttribute('aria-selected', 'true')
        await expect(
          page.locator('#metric-chart-tabpanel').getByRole('status')
        ).toHaveText(message)
        await expect(
          page.locator('.heatmap-cell, .histogram-hover-column')
        ).toHaveCount(0)
      }
      if (readings === 2) {
        const series = page.getByRole('region', {
          name: 'Timeseries',
          exact: true,
        })
        await series.locator('.ts-row').first().click()
        const datapoints = page.getByRole('region', {
          name: 'Selected series datapoints',
        })
        await expect(datapoints.locator('.dp-list__range').first()).toHaveCount(
          0
        )
        const rows = datapoints.locator('tr[data-dp-id]')
        await expect(rows).toHaveCount(2)
        for (const row of await rows.all()) {
          await expect(row.locator('td').nth(1)).toHaveText('12')
          await expect(row.locator('td').nth(2)).toHaveText('18')
        }
      }
    })
  }

  test('empty reduced series load once when newly checked and refresh explicitly', async ({
    page,
  }) => {
    await page.addInitScript(() => {
      localStorage.setItem('metrics:view:storage-version', '1')
      localStorage.setItem(
        'metrics:view:series-layout',
        JSON.stringify({ visibleKeys: ['first'] })
      )
    })
    const requests: { datapointSeriesRefs?: string[] }[] = []
    page.on('request', request => {
      if (!request.url().endsWith('/rpc')) return
      const body = request.postDataJSON()
      if (body.method === 'getMetricView') requests.push(body.params)
    })
    await holdAggregates(page, false, 1)
    const checkbox = page.getByRole('checkbox', {
      name: 'Show route: /second in chart',
      exact: true,
    })
    await expect(checkbox).not.toBeChecked()
    await page.waitForTimeout(400)
    expect(requests).toHaveLength(1)
    expect(requests[0]!.datapointSeriesRefs).toEqual(['first'])
    await checkbox.check()
    await expect.poll(() => requests.length).toBe(2)
    expect(requests[1]!.datapointSeriesRefs).toEqual(['first', 'second'])
    await page.waitForTimeout(400)
    expect(requests).toHaveLength(2)
    await checkbox.uncheck()
    await checkbox.check()
    await page.waitForTimeout(400)
    expect(requests).toHaveLength(2)
    await page.getByRole('button', { name: 'Refresh', exact: true }).click()
    await expect.poll(() => requests.length).toBe(3)
    await page.waitForTimeout(400)
    expect(requests).toHaveLength(3)
  })

  test('selected contributions paint behind intersecting heatmap quantiles', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 1000 })
    await page.emulateMedia({ reducedMotion: 'reduce' })
    const pending = await holdAggregates(page)
    await Promise.all(pending.map(({ route }) => respond(route)))
    const cells = page.locator('.heatmap-cell')
    const quantiles = page.locator('.heatmap-quantile-line')
    await expect(cells).toHaveCount(3)
    await expect(quantiles).toHaveCount(3)
    const cellGeometry = () =>
      cells.evaluateAll(elements =>
        elements.map(element =>
          ['x', 'y', 'width', 'height', 'fill'].map(attr =>
            element.getAttribute(attr)
          )
        )
      )
    const paths = () =>
      quantiles.evaluateAll(elements =>
        elements.map(element => element.getAttribute('d'))
      )
    const originalCells = await cellGeometry()
    const originalPaths = await paths()
    await routeTelemetry(page, async route => {
      const request = route.request().postDataJSON()
      if (request.method !== 'getMetricAggregateView') return route.fallback()
      await route.fulfill({
        json: {
          jsonrpc: '2.0',
          id: request.id,
          result: {
            aggregate: [bucket],
            scalarAggregate: null,
            contribution: [bucket],
          },
        },
      })
    })
    // Route-selected Details datapoints use the same contribution request as a
    // selection in the Details table, without chart pinning changing the source.
    await page.goto(
      `/metrics/${metric.metricRef}?time=all&htab=heatmap&hscope=window&series=first&dp=first-0`
    )
    const textures = page.locator('.heatmap-contribution-texture')
    await expect(textures).toHaveCount(3)
    await expect(
      page.locator('.heatmap-cell[data-contribution-count]')
    ).toHaveCount(3)
    expect(await cellGeometry()).toEqual(originalCells)
    expect(await paths()).toEqual(originalPaths)
    expect(
      await quantiles.first().evaluate(element => {
        const line = element as SVGPathElement
        const point = line.getPointAtLength(line.getTotalLength() / 2)
        const screenPoint = new DOMPoint(point.x, point.y).matrixTransform(
          line.getScreenCTM()!
        )
        const contributions = document.querySelector('.heatmap-contributions')!
        const overlays = line.closest('.heatmap-quantile-overlays')!
        const intersects = [
          ...contributions.querySelectorAll<SVGPathElement>(
            'g[clip-path] > path:first-child'
          ),
        ].some(fill =>
          fill.isPointInFill(
            screenPoint.matrixTransform(fill.getScreenCTM()!.inverse())
          )
        )
        return {
          intersects,
          sameLayer: contributions.parentElement === overlays.parentElement,
          contributionsFirst: Boolean(
            contributions.compareDocumentPosition(overlays) &
            Node.DOCUMENT_POSITION_FOLLOWING
          ),
        }
      })
    ).toEqual({ intersects: true, sameLayer: true, contributionsFirst: true })
    const graph = page.getByRole('application', {
      name: 'Histogram heatmap',
      exact: true,
    })
    await graph.focus()
    await graph.press('ArrowRight')
    await expect(page.locator('.chart-keyboard-cursor-cell')).toBeVisible()
    await graph.press('Enter')
    await expect(page.locator('.heatmap-selection')).toBeVisible()
    expect(await cellGeometry()).toEqual(originalCells)
    expect(await paths()).toEqual(originalPaths)
  })

  for (const { id: theme } of THEMES) {
    const colour = themeRgb(theme, 'muted')
    test(`rounded axes and chart-surface count legends stay legible in ${theme}`, async ({
      page,
    }) => {
      await page.addInitScript(
        theme => localStorage.setItem('theme', theme),
        theme
      )
      await page.setViewportSize({ width: 1440, height: 1000 })
      await page.emulateMedia({ reducedMotion: 'reduce' })
      const pending = await holdAggregates(page)
      for (const { route } of pending) await respond(route)
      for (const tab of ['Heatmap', 'Histogram']) {
        await page.getByRole('tab', { name: tab, exact: true }).click()
        const header = page.getByRole('region', {
          name: 'Metric chart',
          exact: true,
        })
        const chart = page.locator('#metric-chart-tabpanel')
        const legend = chart.getByRole('list', {
          name: `${tab} count scale`,
          exact: true,
        })
        await expect(legend).toBeVisible()
        await expect(
          legend.locator('.heatmap-legend__swatch').first()
        ).toHaveCSS('width', '8px')
        await expect(
          legend.locator('.heatmap-legend__swatch').first()
        ).toHaveCSS('height', '8px')
        await expect(
          legend.locator('.heatmap-legend__swatch').first()
        ).toHaveCSS('border-top-width', '0px')
        await expect(legend).toHaveCSS('font-size', '14px')
        await expect(legend).toHaveCSS('font-weight', '400')
        await expect(legend.getByRole('listitem')).toHaveText([
          '1\u2009–\u20092',
          '3\u2009–\u20094',
          '5\u2009–\u20096',
          '7',
        ])
        expect(
          await legend
            .locator('.heatmap-legend__swatch')
            .evaluateAll(dots =>
              dots.map(dot => getComputedStyle(dot).backgroundColor)
            )
        ).toEqual(countRamp(theme, 4).map(rgb))
        await expect(
          header.locator('[aria-label="Chart time range"]')
        ).toHaveCount(0)
        await expect(header.locator('.heatmap-legend')).toHaveCount(0)
        const legendBox = (await legend.boundingBox())!
        const headerBox = (await header.boundingBox())!
        const plotBox = (await chart
          .locator('.metric-chart-view__plot-host')
          .boundingBox())!
        expect(legendBox.y).toBeGreaterThan(headerBox.y + headerBox.height)
        expect(legendBox.y + legendBox.height).toBeLessThan(plotBox.y)
        const labels = chart.locator('.lc-axis text')
        expect(await labels.count()).toBeGreaterThan(0)
        for (const label of await labels.all())
          await expect(label).toHaveCSS('font-size', '14px')
        await expectRoundedAxes(page, colour)
      }
      await page.setViewportSize({ width: 900, height: 1000 })
      const chart = page.locator('#metric-chart-tabpanel')
      const entries = chart.locator('.legend-rows')
      await expect(entries).toHaveCSS('flex-direction', 'column')
      await expect
        .poll(() =>
          chart.evaluate(e => {
            const chart = e.getBoundingClientRect()
            return [...e.querySelectorAll('.legend-rows__entry')].every(
              entry => {
                const box = entry.getBoundingClientRect()
                return (
                  box.left >= chart.left &&
                  box.right <= chart.right &&
                  box.bottom <= chart.bottom
                )
              }
            )
          })
        )
        .toBe(true)
      await expectRoundedAxes(page, colour)
    })
  }

  test('pinned heatmap descriptions dismiss by pointer and keyboard and can reopen', async ({
    page,
  }) => {
    const pending = await holdAggregates(page)
    for (const { route } of pending) await respond(route)
    const graph = page.getByRole('application', {
      name: 'Histogram heatmap',
      exact: true,
    })
    const card = page.locator('.metric-heatmap-chart .chart-sticky__card')
    const close = page.getByRole('button', {
      name: 'Dismiss pinned bucket description',
    })
    const url = page.url()
    await graph.focus()
    await graph.press('Enter')
    await expect(card).toBeVisible()
    await expect(card.locator('.heatmap-selected-cell')).toBeVisible()
    await close.click()
    await expect(card).toHaveCount(0)
    await expect(page.locator('.heatmap-selection')).toHaveCount(0)
    await expect(graph).toBeFocused()
    expect(page.url()).toBe(url)
    await graph.press('Enter')
    await expect(card).toBeVisible()
    await close.focus()
    await close.press('Enter')
    await expect(card).toHaveCount(0)
    await expect(graph).toBeFocused()
    expect(page.url()).toBe(url)
  })

  test('resizing Details preserves visible heatmap end labels without overriding a scrolled position', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 2400, height: 1000 })
    await page.emulateMedia({ reducedMotion: 'reduce' })
    const pending = await holdAggregates(page)
    const aggregate = Array.from({ length: 100 }, (_, column) => ({
      ...bucket,
      timestamp: String(BigInt(startMs + column * 1000) * 1_000_000n),
    }))
    for (const { route } of pending)
      await route.fulfill({
        json: {
          jsonrpc: '2.0',
          id: route.request().postDataJSON().id,
          result: { aggregate, scalarAggregate: null },
        },
      })
    const viewport = page.locator('.heatmap-scroll')
    const labels = page.locator('.heatmap-quantile-label')
    await expect(labels).toHaveCount(3)
    await expect
      .poll(() => viewport.evaluate(e => e.scrollWidth - e.clientWidth))
      .toBe(0)
    const url = page.url()
    const resize = page
      .getByRole('separator', { name: 'Resize the panels', exact: true })
      .last()
    const handle = (await resize.boundingBox())!
    await page.mouse.move(
      handle.x + handle.width / 2,
      handle.y + handle.height / 2
    )
    await page.mouse.down()
    await page.mouse.move(handle.x - 700, handle.y + handle.height / 2, {
      steps: 20,
    })
    await page.mouse.up()
    await expect
      .poll(() => viewport.evaluate(e => e.scrollLeft))
      .toBeGreaterThan(100)
    await expect
      .poll(() =>
        viewport.evaluate(e => e.scrollWidth - e.clientWidth - e.scrollLeft)
      )
      .toBeLessThanOrEqual(1)
    const bounds = (await viewport.boundingBox())!
    for (const label of await labels.all()) {
      const box = (await label.boundingBox())!
      expect(box.x + box.width).toBeLessThanOrEqual(bounds.x + bounds.width)
    }
    await expect(viewport).toHaveClass(/chart-scroll-discoverable/)
    await viewport.evaluate(e => {
      e.scrollLeft = 30
    })
    await expect.poll(() => viewport.evaluate(e => e.scrollLeft)).toBe(30)
    await page.setViewportSize({ width: 1800, height: 1000 })
    await expect
      .poll(() => viewport.evaluate(e => e.clientWidth))
      .toBeLessThan(bounds.width)
    expect(await viewport.evaluate(e => e.scrollLeft)).toBe(30)
    expect(page.url()).toBe(url)
    await expect(page.locator('.heatmap-cell')).toHaveCount(300)
  })

  test('dense heatmaps join colours without changing cells or adding dot grids', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 2400, height: 1000 })
    const pending = await holdAggregates(page)
    const aggregate = Array.from({ length: 81 }, (_, column) => ({
      ...bucket,
      timestamp: String(BigInt(startMs + column * 1000) * 1_000_000n),
      explicitBounds: [1, 2, 3, 4],
      bucketCounts: Array.from({ length: 5 }, (_, row) =>
        column === 70 || (column === 40 && row === 2)
          ? 0
          : column >= 10 && column <= 11 && row >= 1 && row <= 2
            ? column === 11 && row === 2
              ? 5
              : 4
            : 1 + ((column + row) % 12)
      ),
    }))
    for (const { route } of pending)
      await route.fulfill({
        json: {
          jsonrpc: '2.0',
          id: route.request().postDataJSON().id,
          result: { aggregate, scalarAggregate: null },
        },
      })
    const group = page.locator('.heatmap-blobs')
    await expect(page.locator('.heatmap-cell')).toHaveCount(399)
    const cells = () =>
      page.locator('.heatmap-cell').evaluateAll(elements =>
        elements
          .map(element => {
            const rect = element as SVGRectElement
            return {
              column: Math.round(
                rect.x.baseVal.value / rect.width.baseVal.value
              ),
              row: Math.round(rect.y.baseVal.value / rect.height.baseVal.value),
              fill: rect.getAttribute('fill'),
            }
          })
          .sort((a, b) => a.column - b.column || a.row - b.row)
      )
    await expect(group).not.toHaveClass(/heatmap-blobs--joined/)
    const originalCells = await cells()
    for (const [width, joined] of [
      [1200, true],
      [2400, false],
    ] as const) {
      await page.setViewportSize({ width, height: 1000 })
      await expect
        .poll(() =>
          group.evaluate(element =>
            element.classList.contains('heatmap-blobs--joined')
          )
        )
        .toBe(joined)
      expect(await cells()).toEqual(originalCells)
      if (joined) {
        await expect(page.locator('.heatmap-blob')).toHaveCount(2)
        await expect(
          page.locator(
            '.heatmap-blob > .heatmap-blob-fill,.heatmap-blob > .heatmap-blob-outline'
          )
        ).toHaveCount(0)
        const regions = page.locator(
          '.heatmap-colour-outlines .heatmap-blob-outline'
        )
        await expect(regions).toHaveCount(12)
        const liners = await regions.evaluateAll(elements =>
          elements.map(element => {
            const clipID = element.getAttribute('clip-path')!.slice(5, -1)
            const path = element.getAttribute('d')!
            const probe = document.createElementNS(
              'http://www.w3.org/2000/svg',
              'path'
            )
            const colour =
              getComputedStyle(element).getPropertyValue('--blob-color')
            probe.style.stroke = `color-mix(in oklab, ${colour} 85%, var(--color-base-content))`
            element.parentElement!.append(probe)
            const expectedStroke = getComputedStyle(probe).stroke
            probe.remove()
            return {
              sameBoundary:
                path ===
                document
                  .getElementById(clipID)!
                  .querySelector('path')!
                  .getAttribute('d'),
              fill: getComputedStyle(element).fill,
              stroke: getComputedStyle(element).stroke,
              expectedStroke,
              width: Number.parseFloat(getComputedStyle(element).strokeWidth),
              pointerEvents: getComputedStyle(element).pointerEvents,
            }
          })
        )
        for (const liner of liners) {
          expect(liner.sameBoundary).toBe(true)
          expect(liner.fill).toBe('none')
          expect(liner.stroke).toBe(liner.expectedStroke)
          expect(liner.width).toBeGreaterThan(0)
          expect(liner.pointerEvents).toBe('none')
        }
        const corners = page.locator('.heatmap-colour-corners path')
        expect(await corners.count()).toBeGreaterThan(0)
        for (const corner of await corners.all())
          await expect(corner).toHaveAttribute('d', /Q/)
        await expect(page.locator('.heatmap-joined-outline')).toHaveCount(2)
        const outlines = await page
          .locator('.heatmap-blob')
          .evaluateAll(elements =>
            elements.map(element => {
              const outline = element.querySelector<SVGGElement>(
                '.heatmap-joined-outline'
              )!
              const maskID = outline.getAttribute('mask')!.slice(5, -1)
              const mask = document.getElementById(maskID)!
              const clipID = element.getAttribute('clip-path')!.slice(5, -1)
              const clip = document.getElementById(clipID)!
              return {
                samePerimeter:
                  mask.querySelector('path')!.getAttribute('d') ===
                  clip.querySelector('path')!.getAttribute('d'),
                rings: (
                  mask.querySelector('path')!.getAttribute('d')!.match(/M/g) ??
                  []
                ).length,
                pointerEvents: getComputedStyle(outline).pointerEvents,
                colourWeight: getComputedStyle(outline)
                  .getPropertyValue('--heatmap-cell-colour-weight')
                  .trim(),
                noCellStrokes: [...outline.querySelectorAll('rect')].every(
                  rect => getComputedStyle(rect).stroke === 'none'
                ),
                sameColours: [...outline.querySelectorAll('rect')].every(
                  (rect, index) =>
                    rect.getAttribute('fill') ===
                    element
                      .querySelectorAll('.heatmap-cell')
                      [index]!.getAttribute('fill')
                ),
              }
            })
          )
        expect(outlines.map(outline => outline.rings).sort()).toEqual([1, 2])
        for (const outline of outlines) {
          expect(outline).toMatchObject({
            samePerimeter: true,
            pointerEvents: 'none',
            colourWeight: '85%',
            sameColours: true,
            noCellStrokes: true,
          })
        }
        await expect(page.locator('.heatmap-cell').first()).toHaveCSS(
          'fill-opacity',
          '1'
        )
      } else {
        await expect(page.locator('.heatmap-joined-outline')).toHaveCount(0)
        await expect(page.locator('.heatmap-colour-outlines')).toHaveCount(0)
      }
      await expect(page.locator('.metric-dot-grid')).toHaveCount(0)
    }
  })

  test('count legends keep at most six aligned columns as available width changes', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 2400, height: 1000 })
    const pending = await holdAggregates(page)
    for (const { route } of pending)
      await respond(route, {
        ...bucket,
        count: 78,
        explicitBounds: Array.from({ length: 11 }, (_, index) => index + 1),
        bucketCounts: Array.from({ length: 12 }, (_, index) => index + 1),
      })
    for (const tab of ['Heatmap', 'Histogram']) {
      await page.getByRole('tab', { name: tab, exact: true }).click()
      const legend = page.getByRole('list', {
        name: `${tab} count scale`,
        exact: true,
      })
      await expect(legend.getByRole('listitem')).toHaveCount(12)
      const labels = await legend.getByRole('listitem').allTextContents()
      const dimensions = await legend.evaluate(element => ({
        widths: [...element.querySelectorAll('.legend-rows__entry')].map(
          entry => entry.getBoundingClientRect().width
        ),
        gap: Number.parseFloat(getComputedStyle(element).columnGap),
      }))
      for (const [count, expected] of [
        [12, [6, 6]],
        [9, [6, 6]],
        [6, [6, 6]],
        [5, [5, 5, 2]],
        [3, [3, 3, 3, 3]],
        [12, [6, 6]],
      ] as const) {
        const available = Math.ceil(
          count * Math.max(...dimensions.widths) + (count - 1) * dimensions.gap
        )
        await legend.evaluate((element, width) => {
          element.style.maxWidth = `${width}px`
        }, available)
        await expect
          .poll(() =>
            legend
              .locator('.legend-rows__row')
              .evaluateAll(rows =>
                rows.map(
                  row => row.querySelectorAll('.legend-rows__entry').length
                )
              )
          )
          .toEqual(expected)
        await expect
          .poll(() =>
            legend.evaluate(element => {
              const bounds = element.getBoundingClientRect()
              return [...element.querySelectorAll('.legend-rows__entry')].every(
                entry =>
                  entry.getBoundingClientRect().right <= bounds.right + 0.5
              )
            })
          )
          .toBe(true)
        expect(await legend.getByRole('listitem').allTextContents()).toEqual(
          labels
        )
        const dotColumns = await legend
          .locator('.legend-rows__row')
          .evaluateAll(rows =>
            rows.map(row =>
              [...row.querySelectorAll('.heatmap-legend__swatch')].map(
                dot => dot.getBoundingClientRect().left
              )
            )
          )
        for (const row of dotColumns.slice(1))
          for (const [column, x] of row.entries())
            expect(x).toBeCloseTo(dotColumns[0]![column]!, 1)
      }
      await legend.evaluate(element =>
        element.style.removeProperty('max-width')
      )
    }
  })

  for (const navigation of ['list', 'footer']) {
    test(`initial load and ${navigation} navigation fetch aggregates without advancing the debounce clock`, async ({
      page,
    }) => {
      const now = new Date('2026-10-18T12:00:00Z')
      await page.clock.install({ time: now })
      await page.clock.pauseAt(now)
      const pending = await holdAggregates(page, true)
      if (navigation === 'list') {
        await page
          .getByRole('button', { name: /next\.histogram/ })
          .click({ force: true })
      } else {
        await page
          .getByRole('button', { name: 'Next metric', exact: true })
          .click({ force: true })
      }
      await expect.poll(() => pending.length).toBe(4)
      for (const { route } of pending.slice(2)) {
        expect(route.request().postDataJSON().params.metricRef).toBe(
          `${metric.metricRef}-next`
        )
      }
    })
  }

  test('rapid checkbox changes still coalesce into one request pair after 120ms', async ({
    page,
  }) => {
    const now = new Date('2026-10-18T12:00:00Z')
    await page.clock.install({ time: now })
    await page.clock.pauseAt(now)
    const pending = await holdAggregates(page)
    const checkbox = page.getByRole('checkbox', {
      name: 'Show route: /second in chart',
      exact: true,
    })
    await expect(checkbox).toBeAttached()
    // Allow deferred panel measurements to place the checkbox before clicking.
    await page.clock.runFor(32)
    await expect(checkbox).toBeVisible()
    await checkbox.uncheck({ force: true })
    await page.clock.runFor(60)
    expect(pending).toHaveLength(2)
    await checkbox.check({ force: true })
    await page.clock.runFor(119)
    expect(pending).toHaveLength(2)
    await page.clock.runFor(1)
    await expect.poll(() => pending.length).toBe(4)
    for (const { route } of pending.slice(2)) {
      expect(route.request().postDataJSON().params.seriesRefs).toEqual([
        'first',
        'second',
      ])
    }
  })

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
          page.getByText('No bucket data in range', { exact: true })
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

for (const { id: theme } of THEMES) {
  const hoverColour = themeRgb(theme, 'highlightLow')
  const selectedColour = themeRgb(theme, 'highlightMedium')
  const focusColour = accentRgb(theme)
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
      await routeTelemetry(page, async route => {
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
          ).toHaveCSS('width', '8px')
          expect(await legend.innerText()).not.toMatch(/[()[\]]/)
        }
        const dataFill = await mark.evaluate(
          element => getComputedStyle(element).fill
        )
        const ramp = countRamp(theme, 4)
        const fills =
          tab === 'heatmap' ? await mixedInkColours(page, ramp) : ramp.map(rgb)
        expect(dataFill).toBe(fills[0])
        const marks = page.locator(markSelector)
        await expect(marks).toHaveCount(tab === 'heatmap' ? 6 : 3)
        const expectedFills = [fills[0], fills[3], fills[1]]
        expect(
          await marks.evaluateAll(elements =>
            elements.map(element => getComputedStyle(element).fill).sort()
          )
        ).toEqual(
          (tab === 'heatmap'
            ? [...expectedFills, ...expectedFills]
            : expectedFills
          ).sort()
        )
        const box = (await mark.boundingBox())!
        await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
        const hover = page.locator(hoverSelector)
        await expect(hover).toBeVisible()
        await expect(hover).toHaveCSS('fill', hoverColour)
        await expect(hover).toHaveCSS('fill-opacity', '0.45')
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
          expect(hoveredFill).toBe(
            (await mixedInkColours(page, [ramp[0]!], 88))[0]
          )
        }
        await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2)
        const selection = page.locator(selectionSelector)
        await expect(selection).toBeVisible()
        await expect(selection).toHaveCSS('fill', selectedColour)
        await expect(selection).toHaveCSS('stroke', selectedColour)
        await expect(selection).toHaveCSS('fill-opacity', '0.55')
        await expect(selection).toHaveCSS('rx', '8px')
        await expect(selection).toHaveCSS('pointer-events', 'none')
        if (tab === 'heatmap') {
          await expect(mark).toHaveClass(/heatmap-cell--selected/)
          const selectedFill = await mark.evaluate(
            element => getComputedStyle(element).fill
          )
          expect(selectedFill).not.toBe(dataFill)
          expect(selectedFill).not.toBe(hoveredFill)
          expect(selectedFill).toBe(
            (await mixedInkColours(page, [ramp[0]!], 84))[0]
          )
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
              // At 14px working text the readout can exceed the data plot.
              // Keep it inside the chart viewport in that case, as specified by
              // the heatmap-card-position narrow-pane regression.
              const viewport = (await page
                .locator('.metric-heatmap-chart .chart-sticky')
                .boundingBox())!
              const fitsPlot = cardBox.width <= plot.right - plot.left
              return (
                cardBox.x >= (fitsPlot ? plot.left : viewport.x + 8) - 1 &&
                cardBox.x + cardBox.width <=
                  (fitsPlot ? plot.right : viewport.x + viewport.width - 8) +
                    1 &&
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
          await expect(mark).toHaveCSS(
            'fill',
            (await mixedInkColours(page, [ramp[0]!], 85))[0]!
          )
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
        const quantileColours = [
          'shin-ryoku:6',
          'daikokuten:2',
          'benzaiten:2',
        ].map(id => rgb(graphSwatch(id)!.color))
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
            await expect(channel).toHaveCSS('stroke-width', '4px')
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
        const emphasis = page.getByRole('button', {
          name: 'Emphasize p95 overlay',
          exact: true,
        })
        const urlBeforeEmphasis = page.url()
        const geometry = await quantileLines.evaluateAll(lines =>
          lines.map(line =>
            ['d', 'x1', 'x2', 'y1', 'y2'].map(attr => line.getAttribute(attr))
          )
        )
        await emphasis.click()
        await expect(emphasis).toHaveAttribute('aria-pressed', 'true')
        await expect(quantileLines.nth(1)).toHaveCSS('stroke-width', '4px')
        await expect(quantileLines.nth(1)).toHaveCSS('opacity', '0.95')
        await expect(quantileLines.first()).toHaveCSS('stroke-width', '2px')
        await expect(page.locator('.quantile-line-backing').nth(1)).toHaveCSS(
          'stroke-width',
          '8px'
        )
        if (tab === 'heatmap')
          await expect(
            page.locator('.heatmap-quantile-channel').nth(1)
          ).toHaveCSS('stroke-width', '8px')
        await expect(page).toHaveURL(urlBeforeEmphasis)
        expect(
          await quantileLines.evaluateAll(lines =>
            lines.map(line =>
              ['d', 'x1', 'x2', 'y1', 'y2'].map(attr => line.getAttribute(attr))
            )
          )
        ).toEqual(geometry)
        await emphasis.press('Space')
        await expect(emphasis).toHaveAttribute('aria-pressed', 'false')
        await expect(quantileLines.nth(1)).toHaveCSS('stroke-width', '2px')
        await expect(quantileLines.nth(1)).toHaveCSS('opacity', '0.35')
        await expect(page.locator('.quantile-line-backing').nth(1)).toHaveCSS(
          'stroke-width',
          '4px'
        )
        if (tab === 'heatmap')
          await expect(
            page.locator('.heatmap-quantile-channel').nth(1)
          ).toHaveCSS('stroke-width', '4px')
        const overlaysToggle = page.getByRole('checkbox', {
          name: 'Show Percentiles',
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
      if (theme === 'madoromi-fuyu-yoru' && metricType === 'Histogram') {
        await page
          .getByRole('button', { name: 'Collapse sidebar', exact: true })
          .click()
        await page
          .getByRole('button', { name: 'Collapse details', exact: true })
          .click()
        await page
          .getByRole('checkbox', {
            name: 'Show Percentiles',
            exact: true,
          })
          .uncheck()
        await page.setViewportSize({ width: 390, height: 1000 })
        await expect(
          page.getByRole('checkbox', {
            name: 'Show Percentiles',
            exact: true,
          })
        ).not.toBeChecked()
        await expect(
          page.locator('.pane-header').getByRole('checkbox', {
            name: 'Show Percentiles',
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

test('line stickies stay near their marker and avoid measured stat labels after resizing', async ({
  page,
}) => {
  await routeTelemetry(page, async route => {
    const request = route.request().postDataJSON() as {
      id: number
      method: string
    }
    if (request.method !== 'getMetricView') return route.fallback()
    await route.fulfill({
      json: {
        jsonrpc: '2.0',
        id: request.id,
        result: {
          ...metric,
          timeseries: metric.timeseries.map(series => ({
            ...series,
            stats: { count: 40, min: 1, max: 40, sum: 820, avg: 20.5 },
          })),
        },
      },
    })
  })
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.goto(
    `/metrics/${metric.metricRef}?agg=raw&series=first&dp=first-0`
  )
  const card = page.locator('.chart-sticky__card')
  await expect(card).toBeVisible()
  await expect(
    page.locator('.series-stat-tooltip[title^="min "]')
  ).toBeVisible()
  for (const width of [1920, 1280]) {
    await page.setViewportSize({ width, height: 1000 })
    await expect
      .poll(() =>
        page.evaluate(() => {
          const card = document
            .querySelector('.chart-sticky__card')!
            .getBoundingClientRect()
          const marker = document
            .querySelector('.selection-dot--selected')!
            .getBoundingClientRect()
          const x = marker.left + marker.width / 2
          const y = marker.top + marker.height / 2
          const overlaps = (rect: DOMRect) =>
            card.left < rect.right &&
            card.right > rect.left &&
            card.top < rect.bottom &&
            card.bottom > rect.top
          return {
            labelOverlap: [
              ...document.querySelectorAll('.series-stat-tooltip'),
            ].some(label => overlaps(label.getBoundingClientRect())),
            markerOverlap: overlaps(marker),
            nearby:
              Math.hypot(
                Math.max(card.left - x, x - card.right, 0),
                Math.max(card.top - y, y - card.bottom, 0)
              ) <= 48,
          }
        })
      )
      .toEqual({ labelOverlap: false, markerOverlap: false, nearby: true })
  }
  for (const theme of ['madoromi-aki-hiru', 'madoromi-aki-yoru']) {
    await page.evaluate(theme => {
      document.documentElement.setAttribute('data-theme', theme)
      document.documentElement.setAttribute(
        'data-color-scheme',
        theme.endsWith('hiru') ? 'light' : 'dark'
      )
    }, theme)
    expect(
      await card.locator('.chart-selection-legend').evaluate(element => {
        const probe = document.createElement('div')
        probe.className = 'popover-surface'
        document.body.append(probe)
        const actual = getComputedStyle(element)
        const expected = getComputedStyle(probe)
        const matches =
          actual.backgroundColor === expected.backgroundColor &&
          actual.border === expected.border &&
          actual.boxShadow === expected.boxShadow
        probe.remove()
        return matches
      })
    ).toBe(true)
  }
})

test('extrema use the series colour and the backed average stays behind the series without a data dot', async ({
  page,
}) => {
  await page.addInitScript(() =>
    localStorage.setItem('theme', 'madoromi-aki-hiru')
  )
  await routeTelemetry(page, async route => {
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
  await expect(average.locator('.series-average-line-backing')).toHaveCSS(
    'stroke-width',
    '4px'
  )
  const averageLabel = page.locator('.series-stat-tooltip--right')
  await expect(
    averageLabel.locator('.chart-selection-legend__label')
  ).toHaveText('Series avg')
  await expect(averageLabel).toHaveAttribute('title', 'Series avg 20.5')
  await expect(
    averageLabel.locator('.chart-selection-legend--stat')
  ).toBeVisible()
  await expect(average.locator('.series-average-line')).toHaveCSS(
    'stroke-dasharray',
    '6px, 4px'
  )
  await expect(average.locator('.series-average-line')).toHaveCSS(
    'stroke-width',
    '2px'
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
      const maxLabelBox = chart
        .querySelector('.series-stat-tooltip[title^="max "]')!
        .getBoundingClientRect()
      const plotBox = chart
        .querySelector('.metric-chart-plot')!
        .getBoundingClientRect()
      return {
        maxLabelFits:
          maxLabelBox.top >= plotBox.top &&
          maxLabelBox.bottom <= plotBox.bottom,
        extremaMatch: minColour === maxColour,
        seriesFound: !!series,
        averageAboveSeries:
          !!series &&
          !!(
            reference.compareDocumentPosition(series) &
            Node.DOCUMENT_POSITION_PRECEDING
          ),
        averageHasOwnColour: getComputedStyle(reference).stroke !== minColour,
        averageLabelMatches:
          getComputedStyle(reference).stroke ===
          getComputedStyle(label).backgroundColor,
        averageColour: getComputedStyle(reference).stroke,
        averageLabelAtRight: Math.abs(labelBox.left - lineBox.right - 8) < 1,
      }
    })
  expect(drawing).toEqual({
    maxLabelFits: true,
    extremaMatch: true,
    seriesFound: true,
    averageAboveSeries: false,
    averageHasOwnColour: true,
    averageLabelMatches: true,
    averageColour: accentRgb('madoromi-aki-hiru'),
    averageLabelAtRight: true,
  })
  const emphasis = averageLabel.getByRole('button', {
    name: 'Emphasize Series avg overlay',
  })
  const before = page.url()
  await expect(emphasis).toHaveCSS('font-weight', '400')
  for (const text of await averageLabel
    .locator('.chart-selection-legend__label, .chart-selection-legend__value')
    .all()) {
    await expect(text).toHaveCSS('font-weight', '400')
  }
  await emphasis.click()
  await expect(emphasis).toHaveCSS('font-weight', '400')
  await expect(average.locator('.series-average-line-backing')).toHaveCSS(
    'stroke-width',
    '8px'
  )
  await expect(average.locator('.series-average-line')).toHaveCSS(
    'stroke-width',
    '4px'
  )
  await expect(average.locator('.series-average-line')).toHaveCSS(
    'opacity',
    '0.95'
  )
  await expect(page).toHaveURL(before)
  await emphasis.click()
  await expect(average.locator('.series-average-line')).toHaveCSS(
    'stroke-width',
    '2px'
  )
  await expect(average.locator('.series-average-line')).toHaveCSS(
    'opacity',
    '0.35'
  )
  for (const [width, fontSize] of [
    [1200, 16],
    [1920, 24],
  ] as const) {
    await page.setViewportSize({ width, height: 1100 })
    await page.locator('html').evaluate((element, size) => {
      element.style.fontSize = `${size}px`
    }, fontSize)
    await expect
      .poll(() =>
        averageLabel.evaluate(element => {
          const label = element.getBoundingClientRect()
          const plot = element
            .closest('.metric-time-series-chart__plot')!
            .getBoundingClientRect()
          return plot.right - label.right
        })
      )
      .toBeGreaterThanOrEqual(7.5)
    await expect(emphasis).toHaveText('Series avg')
    await expect(emphasis).toHaveCSS('font-size', `${fontSize * 0.875}px`)
  }
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
  await routeTelemetry(page, async route => {
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
    page.getByRole('tab', { name: 'Values', exact: true })
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
  ).toHaveCount(0)
})

test('combined average labels avoid source statistics while staying near their endpoints', async ({
  page,
}) => {
  const buckets = [0, 20, 39].map(index => ({
    bucketStart: String(BigInt(startMs + index * 1000) * 1_000_000n),
    sampleCount: 2,
    sum: 20,
    avg: 10,
    rate: null,
    slope: null,
    hasReset: false,
  }))
  await routeTelemetry(page, async route => {
    const { id, method } = route.request().postDataJSON()
    const result =
      method === 'getMetricView'
        ? {
            ...metric,
            timeseries: timeseries.map(series => ({
              ...series,
              datapoints: series.datapoints.map(point => ({
                ...point,
                doubleValue: 10,
              })),
              stats: { count: 40, min: 10, max: 10, sum: 400, avg: 10 },
            })),
          }
        : method === 'getMetricAggregateView'
          ? {
              aggregate: null,
              scalarAggregate: { all: buckets, selected: buckets },
            }
          : undefined
    if (result === undefined) await route.fallback()
    else await route.fulfill({ json: { jsonrpc: '2.0', id, result } })
  })
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.goto(`/metrics/${metric.metricRef}?agg=avg&series=first`)
  const average = page.locator('[data-aggregate-label="__agg:avg:total__"]')
  await expect(average).toBeVisible()
  for (const width of [1920, 1280]) {
    await page.setViewportSize({ width, height: 1000 })
    await expect
      .poll(() =>
        average.evaluate(element => {
          const rect = element.getBoundingClientRect()
          const stat = document
            .querySelector('.series-stat-tooltip[title^="Series avg "]')!
            .getBoundingClientRect()
          return {
            overlap:
              rect.left < stat.right &&
              rect.right > stat.left &&
              rect.top < stat.bottom &&
              rect.bottom > stat.top,
            nearby:
              Math.hypot(
                Math.max(
                  rect.left - (stat.left - 8),
                  stat.left - 8 - rect.right,
                  0
                ),
                Math.max(
                  rect.top - (stat.top + stat.height / 2),
                  stat.top + stat.height / 2 - rect.bottom,
                  0
                )
              ) <= 48,
          }
        })
      )
      .toEqual({ overlap: false, nearby: true })
  }
  await average.getByRole('button').click()
  await expect(average.getByRole('button')).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  const sourceLabel = page
    .locator('.series-stat-tooltip[title^="Series avg "]')
    .getByRole('button')
  await expect(sourceLabel).toHaveAttribute('aria-pressed', 'false')
  await sourceLabel.click()
  await expect(sourceLabel).toHaveAttribute('aria-pressed', 'true')
  expect(new URL(page.url()).searchParams.get('series')).toBe('first')
  expect(new URL(page.url()).searchParams.has('dp')).toBe(false)
})

test('combined sum and average toggle independently and labels emphasize without changing inspection', async ({
  page,
}) => {
  const counterMetadata = {
    metricType: 'Sum',
    isMonotonic: true,
    aggregationTemporalityCode: 2,
    aggregationTemporality: 'Cumulative',
  }
  const buckets = [0, 20, 39].map(index => ({
    bucketStart: String(BigInt(startMs + index * 1000) * 1_000_000n),
    sampleCount: 2,
    sum: (index + 1) * 2,
    avg: index + 1,
    rate: null,
    slope: null,
    hasReset: false,
  }))
  await routeTelemetry(page, async route => {
    const { id, method } = route.request().postDataJSON()
    const result =
      method === 'getMetricView'
        ? {
            ...metric,
            ...counterMetadata,
            timeseries: timeseries.map(series => ({
              ...series,
              datapoints: series.datapoints.map(point => ({
                ...point,
                ...counterMetadata,
              })),
            })),
          }
        : method === 'searchMetricSummaries'
          ? [{ ...summary, ...counterMetadata }]
          : method === 'getMetricAggregateView'
            ? {
                aggregate: null,
                scalarAggregate: { all: buckets, selected: buckets },
              }
            : undefined
    if (result === undefined) await route.fallback()
    else await route.fulfill({ json: { jsonrpc: '2.0', id, result } })
  })
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.goto(
    `/metrics/${metric.metricRef}?agg=raw&series=first&dp=first-1`
  )
  const sum = page.getByRole('checkbox', {
    name: 'Show combined sum',
    exact: true,
  })
  const average = page.getByRole('checkbox', {
    name: 'Show combined average',
    exact: true,
  })
  const source = page.locator('.metric-series-line[data-series-key="first"]')
  const sourceColour = await source.evaluate(
    element => getComputedStyle(element).stroke
  )
  await sum.check()
  await average.check()
  await expect(page.locator('[data-aggregate-label]')).toHaveCount(2)
  await expect(page).toHaveURL(/agg=sum-avg/)
  const sumLine = page.locator(
    '.metric-series-line[data-series-key="__agg:total__"]'
  )
  const avgLine = page.locator(
    '.metric-series-line[data-series-key="__agg:avg:total__"]'
  )
  await expect(sumLine).toHaveCSS('stroke-width', '2px')
  await page
    .locator('[data-aggregate-label="__agg:total__"]')
    .getByRole('button')
    .click()
  await expect(sumLine).toHaveCSS('stroke-width', '4px')
  await expect(avgLine).toHaveCSS('stroke-width', '2px')
  expect(new URL(page.url()).searchParams.get('dp')).toBe('first-1')
  expect(new URL(page.url()).searchParams.get('series')).toBe('first')
  await expect(source).toHaveCSS('stroke', sourceColour)
  await sum.uncheck()
  await expect(sumLine).toHaveCount(0)
  await expect(avgLine).toBeVisible()
  await expect(average).toBeChecked()
  await page.reload()
  await expect(average).toBeChecked()
  await expect(sum).not.toBeChecked()
  await average.uncheck()
  await expect(page.locator('[data-aggregate-label]')).toHaveCount(0)
})

test('Rate clicks pin the exact bucket and put slope at the right edge', async ({
  page,
}) => {
  await page.addInitScript(() =>
    localStorage.setItem('theme', 'madoromi-aki-hiru')
  )
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
  await routeTelemetry(page, async route => {
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
  await page.locator('.ts-row[data-series-key]').first().click()
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
  const slopeLabel = page.locator('.series-stat-tooltip--right')
  await expect(slopeLabel.getByText('slope', { exact: true })).toBeVisible()
  const slopeGuide = page.locator('.series-slope-line')
  await expect(slopeGuide).toHaveCSS('stroke-dasharray', '6px, 4px')
  await expect(slopeGuide).toHaveCSS('stroke-width', '2px')
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
      const labelBox = chart
        .querySelector('.series-stat-tooltip--right')!
        .getBoundingClientRect()
      const plotBox = chart
        .querySelector('.metric-chart-plot')!
        .getBoundingClientRect()
      return {
        labelFits: labelBox.right <= plotBox.right,
        aboveSeries: !!(
          guide.compareDocumentPosition(series) &
          Node.DOCUMENT_POSITION_PRECEDING
        ),
        coversIncomingBucket:
          Math.hypot(x2 - x1, y2 - y1) >=
          Math.hypot(px - firstBucket.x, py - firstBucket.y) - 0.01,
        distanceFromPoint:
          Math.abs((px - x1) * (y2 - y1) - (py - y1) * (x2 - x1)) /
          Math.hypot(x2 - x1, y2 - y1),
        distanceFromEnd: Math.hypot(x2 - px, y2 - py),
        colour: getComputedStyle(guide).stroke,
        cardColour: getComputedStyle(
          chart.querySelector(
            '.series-stat-tooltip--right .chart-selection-legend__dot'
          )!
        ).backgroundColor,
      }
    }, firstBucket)
  expect(slopeDrawing.aboveSeries).toBe(false)
  expect(slopeDrawing.labelFits).toBe(true)
  expect(slopeDrawing.coversIncomingBucket).toBe(true)
  expect(slopeDrawing.distanceFromPoint).toBeLessThan(0.01)
  expect(slopeDrawing.distanceFromEnd).toBeLessThan(0.01)
  expect(slopeDrawing.colour).toBe(slopeDrawing.cardColour)
  expect(slopeDrawing.colour).toBe(accentRgb('madoromi-aki-hiru'))
  const slopeEmphasis = slopeLabel.getByRole('button', {
    name: 'Emphasize slope overlay',
  })
  const urlBeforeEmphasis = page.url()
  await slopeEmphasis.click()
  await expect(slopeGuide).toHaveCSS('stroke-width', '4px')
  await expect(slopeGuide).toHaveCSS('opacity', '0.95')
  await expect(page).toHaveURL(urlBeforeEmphasis)
  await slopeEmphasis.press('Enter')
  await expect(slopeGuide).toHaveCSS('stroke-width', '2px')
  await expect(slopeGuide).toHaveCSS('opacity', '0.35')
  await expect(
    page.locator('.series-stat-tooltip').filter({ hasText: 'slope' })
  ).toHaveCount(1)
  const selected = page.locator('.selection-dot--selected')
  const box = (await selected.boundingBox())!
  const x = box.x + box.width / 2
  const y = box.y + box.height / 2
  await graph.press('Escape')
  await expect(sticky).toHaveCount(0)
  await page.mouse.move(x, y)
  await expect(page.locator('.lc-tooltip-header').first()).toBeVisible()
  await page.mouse.click(x, y)
  await expect(slopeLabel.getByText('slope', { exact: true })).toBeVisible()
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
  await page
    .getByRole('checkbox', {
      name: 'Show route: /second in chart',
      exact: true,
    })
    .uncheck()
  await page
    .getByRole('button', { name: 'Select route: /first', exact: true })
    .click()
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

test('line clicks and series rows share selection without changing visibility or disclosure', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.goto(`/metrics/${metric.metricRef}?agg=raw`)
  const lines = page.locator('.metric-series-line')
  await expect(lines).toHaveCount(2)
  const second = page.locator('.metric-series-line[data-series-key="second"]')
  // The two fixture series overlap exactly. A direct hit belongs to the
  // topmost line, not the first series found at the nearest sample.
  const point = await second.evaluate(element => {
    const path = element as SVGPathElement
    const position = path.getPointAtLength(path.getTotalLength() * 0.4)
    const screen = new DOMPoint(position.x, position.y).matrixTransform(
      path.getScreenCTM()!
    )
    return { x: screen.x, y: screen.y }
  })
  await page.mouse.click(point.x, point.y)
  const rows = page.locator('.ts-row')
  await expect(
    rows.filter({
      has: page.getByRole('button', {
        name: 'Select route: /second',
        exact: true,
      }),
    })
  ).toHaveAttribute('aria-selected', 'true')
  await expect(second).toHaveAttribute('data-emphasized', 'true')
  await expect(page).toHaveURL(/series=second/)
  await page
    .getByRole('button', { name: 'Select route: /first', exact: true })
    .click()
  await page.mouse.move(0, 0)
  await expect(
    rows.filter({
      has: page.getByRole('button', {
        name: 'Select route: /first',
        exact: true,
      }),
    })
  ).toHaveAttribute('aria-selected', 'true')
  await expect(
    page.locator('.metric-series-line[data-series-key="first"]')
  ).toHaveAttribute('data-emphasized', 'true')
  await expect(second).toHaveAttribute('data-emphasized', 'false')
  await expect(lines).toHaveCount(2)
  await expect(
    page.getByRole('checkbox', {
      name: 'Show route: /first in chart',
      exact: true,
    })
  ).toBeChecked()
  await expect(
    page.getByRole('checkbox', {
      name: 'Show route: /second in chart',
      exact: true,
    })
  ).toBeChecked()
})

test('chart selection preserves section disclosure and the inspected series', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.goto(`/metrics/${metric.metricRef}?agg=raw`)
  const details = page.getByRole('region', {
    name: 'Metric details',
    exact: true,
  })
  const series = details.locator('[data-detail-section="series"] details')
  const fields = details.locator('[data-detail-section="metric"] details')
  const datapoints = details.locator(
    '[data-detail-section="datapoints"] details'
  )
  await page
    .getByRole('button', { name: 'Select route: /first', exact: true })
    .click()
  await series.locator(':scope > summary').click()
  await expect(series).not.toHaveAttribute('open')
  await fields
    .locator(':scope > summary')
    .getByText('Metric', { exact: true })
    .click()
  await expect(fields).toHaveAttribute('open')
  const graph = page.getByRole('application', {
    name: 'Metric time series chart',
    exact: true,
  })
  await graph.focus()
  await graph.press('Home')
  await graph.press('Enter')
  await expect(series).not.toHaveAttribute('open')
  await expect(fields).toHaveAttribute('open')
  const selectedID = new URL(page.url()).searchParams.get('dp')!
  const selected = datapoints.locator(`[data-dp-id="${selectedID}"]`)
  await expect(selected).toHaveAttribute('aria-selected', 'true')
  await expect(selected).toBeInViewport()
  // Metric remains primary; chart picks do not reopen other sections.
  await fields.locator(':scope > summary').press('Enter')
  await fields.locator(':scope > summary').press('Space')
  await expect(fields).toHaveAttribute('open')
  await graph.focus()
  await graph.press('Enter')
  await expect(series).not.toHaveAttribute('open')
  await expect(selected).toBeInViewport()
  await expect(fields).toHaveAttribute('open')
})

test('dense pinned legends scroll by wheel and keyboard at the 280px chart floor', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.goto(`/metrics/${metric.metricRef}?agg=raw`)
  // Mount the real shared card with a dense readout at the supported chart floor.
  // Raw currently pins only one series; this isolates overflow from selection policy.
  await page.evaluate(async () => {
    const path = '/src/test/DenseChartStickyHarness.svelte'
    const { default: Harness, mount } = await import(path)
    mount(Harness, { target: document.body })
  })
  await expect(page.locator('.dense-chart-fixture')).toHaveCSS(
    'height',
    '280px'
  )
  const card = page.getByRole('region', {
    name: 'Pinned chart selection',
    exact: true,
  })
  await expect(card).toBeVisible()
  await expect(card).toHaveCSS('pointer-events', 'auto')
  await expect(page.locator('.chart-sticky')).toHaveCSS(
    'pointer-events',
    'none'
  )
  expect(
    await card.evaluate(element => element.scrollHeight > element.clientHeight)
  ).toBe(true)
  const selectedURL = page.url()
  await card.hover()
  await page.mouse.wheel(0, 160)
  await expect
    .poll(() => card.evaluate(element => element.scrollTop))
    .toBeGreaterThan(0)
  await card.focus()
  // Chromium can round scrollTop to zero before its Home animation finishes.
  // Wait for native completion before sending the opposite scroll command.
  const scrollKey = async (key: 'Home' | 'End') => {
    const finished = card.evaluate(
      element =>
        new Promise<void>(resolve => {
          element.addEventListener('scrollend', () => resolve(), { once: true })
        })
    )
    await card.press(key)
    await finished
  }
  await scrollKey('Home')
  await expect.poll(() => card.evaluate(element => element.scrollTop)).toBe(0)
  await scrollKey('End')
  await expect
    .poll(() =>
      card.evaluate(
        element =>
          element.scrollTop + element.clientHeight >= element.scrollHeight - 1
      )
    )
    .toBe(true)
  await expect(
    card.locator('.chart-selection-legend__value').last()
  ).toBeInViewport()
  await expect(page).toHaveURL(selectedURL)
  await expect(
    page.getByRole('button', { name: 'Plot selections: 0', exact: true })
  ).toBeVisible()
  // The overlay outside the card must still hit the underlying chart.
  expect(
    await page.locator('.chart-sticky').evaluate(element => {
      const box = element.getBoundingClientRect()
      const hit = document.elementFromPoint(box.left + 2, box.top + 2)
      return hit !== element && !element.contains(hit)
    })
  ).toBe(true)
  await page.mouse.click(52, 102)
  await expect(
    page.getByRole('button', { name: 'Plot selections: 1', exact: true })
  ).toBeVisible()
})

test('series selection drives a separate datapoint section with independent attributes and visibility', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.goto(`/metrics/${metric.metricRef}?agg=raw`)
  const details = page.getByRole('region', {
    name: 'Metric details',
    exact: true,
  })
  const seriesBubble = details.locator(
    '[data-detail-section="series"] details.collapse'
  )
  const fieldsBubble = details.locator(
    '[data-detail-section="metric"] details.collapse'
  )
  const datapointsBubble = details.locator(
    '[data-detail-section="datapoints"] details.collapse'
  )
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
  await expect(
    page.getByRole('group', { name: 'Chart controls', exact: true })
  ).toHaveCount(0)
  await expect(mainHeader).toHaveCSS('border-radius', '12px')
  const range = mainHeader.locator('[aria-label="Chart time range"]')
  await expect(
    mainHeader.getByRole('checkbox', {
      name: 'Show combined average',
      exact: true,
    })
  ).toBeVisible()
  await expect(
    page
      .locator('.metric-chart-view')
      .getByRole('checkbox', { name: 'Show combined average' })
  ).toHaveCount(0)
  await expect(range).toHaveCount(0)
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
    const title = header
      .querySelector('.pane-header__title')!
      .getBoundingClientRect()
    const download = header
      .querySelector('[aria-label="Download metric"]')!
      .getBoundingClientRect()
    const statistics = header
      .querySelector('[aria-label="Show combined average"]')!
      .getBoundingClientRect()
    return {
      statisticsBeforeDownload: statistics.right <= download.left,
      badgesAfterTitle: badges.left >= title.right,
      badgesBeforeDownload: badges.right <= download.left,
      downloadInset: header.getBoundingClientRect().right - download.right,
      titleCenter: top.top + top.height / 2,
      statisticsCenter: statistics.top + statistics.height / 2,
    }
  })
  expect(headerLayout.badgesAfterTitle).toBe(true)
  expect(headerLayout.statisticsBeforeDownload).toBe(true)
  expect(headerLayout.badgesBeforeDownload).toBe(true)
  expect(headerLayout.downloadInset).toBeCloseTo(16, 0)
  expect(headerLayout.statisticsCenter).toBeCloseTo(headerLayout.titleCenter, 0)
  await expect(mainHeader.getByRole('tablist')).toHaveCount(0)
  await expect(mainHeader).toHaveCSS('height', '48px')
  await expect(mainHeader).toHaveCSS(
    'background-color',
    await fieldsBubble
      .locator(':scope > summary')
      .evaluate(element => getComputedStyle(element).backgroundColor)
  )
  const series = page.locator('.ts-row[data-series-key]')
  const seriesDetails = page.locator('[data-series-attributes]')
  await expect(series).toHaveCount(2)
  for (const item of await series.all()) {
    await expect(item).toHaveClass(/item-state/)
    await expect(item).toHaveAttribute('aria-selected', 'false')
  }
  const firstCheckbox = page.getByRole('checkbox', {
    name: 'Show route: /first in chart',
  })
  await firstCheckbox.uncheck()
  await expect(series.nth(0)).toHaveAttribute('aria-selected', 'false')
  await firstCheckbox.check()
  await expect(series.nth(0)).toHaveAttribute('aria-selected', 'false')
  await expect(
    series.nth(0).locator('td').last().getByRole('checkbox')
  ).toBeChecked()
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
    await expect(icon).toHaveClass(/text-subtle/)
  }
  await expect(
    details.locator('.collapse-arrow, .field-group__caret')
  ).toHaveCount(0)
  await series.nth(0).locator('.ts-row__attrs').click()
  await expect(series.nth(0)).toHaveAttribute('aria-selected', 'true')
  await expect(seriesDetails).toHaveCount(0)
  await page
    .getByRole('button', {
      name: 'Expand attributes for route: /first',
      exact: true,
    })
    .click()
  await expect(seriesDetails.nth(0).locator('table')).toHaveCount(1)
  const table = datapointsBubble.getByRole('table', {
    name: 'Datapoints',
    exact: true,
  })
  const attribute = seriesDetails.nth(0).locator('[data-field="route"]')
  await expect(attribute.locator('.detail-property__key')).toHaveText(
    'route · string'
  )
  await expect(attribute.locator('.detail-property__type')).toHaveText('string')
  const fieldLayout = await attribute
    .locator('.detail-property__card')
    .evaluate(card => {
      const header = card.querySelector('.detail-property__header')!
      const key = header
        .querySelector('.detail-property__key')!
        .getBoundingClientRect()
      const type = header
        .querySelector('.detail-property__type')!
        .getBoundingClientRect()
      const copy = card
        .querySelector('.detail-property__header button')!
        .getBoundingClientRect()
      return {
        keyRight: key.right,
        typeInsideHeading: type.left >= key.left && type.right <= key.right,
        headingTop: header
          .querySelector('.detail-property__heading')!
          .getBoundingClientRect().top,
        copyTop: copy.top,
        copyLeft: copy.left,
        copyInset: header.getBoundingClientRect().right - copy.right,
      }
    })
  expect(fieldLayout.typeInsideHeading).toBe(true)
  expect(fieldLayout.copyLeft - fieldLayout.keyRight).toBeGreaterThanOrEqual(8)
  expect(fieldLayout.copyInset).toBeCloseTo(0, 0)
  expect(fieldLayout.copyTop).toBeCloseTo(fieldLayout.headingTop, 0)
  await expect(seriesDetails.nth(0)).toContainText('/first')
  await expect(table.locator('[data-field="route"]')).toHaveCount(0)
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
  await expect(sticky).toHaveCSS('pointer-events', 'auto')
  expect(
    await sticky.evaluate(element => {
      const box = element.getBoundingClientRect()
      const hit = document.elementFromPoint(
        box.x + box.width / 2,
        box.y + box.height / 2
      )
      return !!hit && element.contains(hit)
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
  const nextPage = datapointsBubble.getByRole('button', {
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
  await expect(datapointsBubble).toContainText('26-40 of 40 datapoints')
  await expect(nextPage).toBeDisabled()
  await datapointsBubble
    .getByRole('button', { name: 'First page', exact: true })
    .click()
  await expect(table.locator('tr[data-dp-id]').first()).toHaveAttribute(
    'data-dp-id',
    'first-0'
  )
  await page.setViewportSize({ width: 1440, height: 720 })
  await expect(nextPage).toBeInViewport({ ratio: 1 })
  await datapointsBubble
    .getByRole('combobox', { name: 'Rows per page' })
    .selectOption('50')
  await expect(table.locator('tr[data-dp-id]')).toHaveCount(40)
  await expect(nextPage).toBeInViewport({ ratio: 1 })
  // Section allocation stays stable when more vertical room becomes available.
  const sectionRatio = async () => {
    const seriesHeight =
      (await seriesBubble.boundingBox())!.height -
      (await seriesBubble.locator(':scope > summary').boundingBox())!.height
    const fieldsHeight =
      (await fieldsBubble.boundingBox())!.height -
      (await fieldsBubble.locator(':scope > summary').boundingBox())!.height
    return seriesHeight / (seriesHeight + fieldsHeight)
  }
  const beforeResize = await sectionRatio()
  await page.setViewportSize({ width: 1920, height: 2400 })
  await expect(series.nth(0).locator('details')).toHaveCount(0)
  await expect(series.nth(0)).not.toContainText('service.name')
  const resource = fieldsBubble.locator('[data-field="service.name"]')
  await expect(resource.locator('.detail-property__key')).toHaveText(
    'service.name · string'
  )
  await expect(resource.locator('.detail-property__origin')).toHaveText(
    'resource'
  )
  await expect.poll(sectionRatio).toBeCloseTo(beforeResize, 2)
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page
    .getByRole('checkbox', { name: 'Show route: /first in chart' })
    .uncheck()
  await expect(series.nth(0)).toHaveAttribute('aria-selected', 'true')
  await expect(datapoint).toHaveAttribute('aria-selected', 'true')
  await series.nth(1).locator('.ts-row__attrs').click()
  await expect(series.nth(1)).toHaveAttribute('aria-selected', 'true')
  await expect(series.nth(0)).toHaveAttribute('aria-selected', 'false')
  await expect(seriesDetails.locator('table:visible')).toHaveCount(1)
  await expect(table.locator('tr[data-dp-id]').first()).toHaveAttribute(
    'data-dp-id',
    'second-0'
  )
  await expect(page).not.toHaveURL(/[?&]dp=/)
  await series.nth(0).focus()
  await page.keyboard.press('Enter')
  await expect(series.nth(0)).toHaveAttribute('aria-selected', 'true')
  await expect(series.nth(1)).toHaveAttribute('aria-selected', 'false')
  await expect(firstCheckbox).not.toBeChecked()
})

test('header tabs use icons and tooltips only when their panel is narrow', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1000 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.route('**/export/metrics/**', route =>
    route.fulfill({
      contentType: 'application/json',
      body: '{"resourceMetrics":[]}',
    })
  )
  await page.goto(`/metrics/${metric.metricRef}?agg=raw`)
  const bubble = page
    .getByRole('region', { name: 'Metric details', exact: true })
    .locator('[data-detail-section="metric"]')
  const properties = bubble.getByRole('tab', {
    name: 'Properties',
    exact: true,
  })
  const json = bubble.getByRole('tab', { name: 'JSON', exact: true })
  const labelWidth = () =>
    properties
      .locator('.header-tab__label')
      .evaluate(element => element.getBoundingClientRect().width)
  const tooltipDisplay = () =>
    properties.evaluate(
      element => getComputedStyle(element, '::before').display
    )
  await expect.poll(labelWidth).toBeGreaterThan(20)
  await properties.hover()
  expect(await tooltipDisplay()).toBe('none')

  const resize = page.getByRole('separator', {
    name: 'Resize the panels',
    exact: true,
  })
  await resize.focus()
  await resize.press('Shift+ArrowRight')
  await resize.press('Shift+ArrowRight')
  await expect.poll(labelWidth).toBe(1)
  await expect(properties).toHaveAccessibleName('Properties')
  await expect(json).toHaveAccessibleName('JSON')
  const titleBox = (await bubble
    .locator('summary')
    .getByText('Metric', { exact: true })
    .boundingBox())!
  const controlsBox = (await bubble
    .getByRole('group', { name: 'Detail controls' })
    .boundingBox())!
  expect(controlsBox.x).toBeGreaterThan(titleBox.x + titleBox.width)
  await properties.hover()
  await expect
    .poll(() =>
      properties.evaluate(
        element => getComputedStyle(element, '::before').opacity
      )
    )
    .toBe('1')
  expect(await tooltipDisplay()).toBe('block')
  await properties.focus()
  await page.keyboard.press('ArrowRight')
  await expect(json).toBeFocused()
  await expect(json).toHaveAttribute('aria-selected', 'true')

  await resize.focus()
  await resize.press('Shift+ArrowLeft')
  await resize.press('Shift+ArrowLeft')
  await expect.poll(labelWidth).toBeGreaterThan(20)
  expect(await tooltipDisplay()).toBe('none')
  await expect(json).toHaveAttribute('aria-selected', 'true')
})

test('metric JSON loads without datapoints or a Load button while Copy and Download keep the complete metric', async ({
  page,
  context,
}) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.setViewportSize({ width: 1920, height: 1000 })
  const metadata =
    '{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"request.duration","gauge":{}}]}]}]}'
  const complete =
    '{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"request.duration","gauge":{"dataPoints":[{"asInt":"9223372036854775807"}]}}]}]}]}'
  const requests: string[] = []
  await page.route('**/export/metrics/**', async route => {
    const url = new URL(route.request().url())
    const text =
      url.searchParams.get('includeDatapoints') === 'false'
        ? metadata
        : complete
    requests.push(`${route.request().method()} ${url.search}`)
    await route.fulfill({
      contentType: 'application/json',
      headers: { 'Content-Length': String(text.length) },
      body: route.request().method() === 'HEAD' ? '' : text,
    })
  })
  await page.goto(`/metrics/${metric.metricRef}?agg=raw`)
  const detail = page.getByRole('region', {
    name: 'Metric details',
    exact: true,
  })
  const bubble = detail.locator('[data-detail-section="metric"]')
  await bubble.getByRole('tab', { name: 'JSON', exact: true }).click()
  const json = bubble.getByRole('region', {
    name: 'Raw JSON inspector',
    exact: true,
  })
  await expect(json.locator('.cm-content')).toContainText('request.duration')
  await expect(json.locator('.cm-content')).not.toContainText('dataPoints')
  await expect(json.getByRole('button', { name: /^Load/ })).toHaveCount(0)
  expect(requests).toEqual([
    'HEAD ?includeDatapoints=false',
    'GET ?includeDatapoints=false',
  ])
  await bubble.getByRole('button', { name: 'Copy metric', exact: true }).click()
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(complete)
  expect(requests.at(-1)).toBe('GET ')
  const download = page.waitForEvent('download')
  await page
    .getByRole('button', { name: 'Download metric', exact: true })
    .click()
  expect(await (await download).failure()).toBeNull()
  expect(requests.at(-1)).toBe('GET ')
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
  const bubble = detail.locator('[data-detail-section="metric"]')
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
  ).toBeCloseTo(16, 0)
  await heading.getByText('Metric', { exact: true }).click()
  await expect(bubble.locator('details')).toHaveAttribute('open')
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
  await expect(bubble.locator('details')).toHaveAttribute('open')
  await expect(page).toHaveURL(`/metrics/${metric.metricRef}?agg=raw`)
})
