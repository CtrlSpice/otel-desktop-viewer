import { expect, test } from '@playwright/test'

test('service inspection scopes the counted Telemetry tabs with independent 25-card pages', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1100 })
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  const serviceIdentity = JSON.stringify([
    { kind: 'string', value: 'checkout' },
    { kind: 'int64', value: '7' },
  ])
  const traceID = '11111111111111111111111111111111'
  const spanID = '0000000000000001'
  const otherSpanID = '0000000000000002'
  const requests: { method: string; params: Record<string, unknown> }[] = []
  await page.addInitScript(() => {
    localStorage.setItem('signal-drawer:open', 'true')
    localStorage.setItem('signal-detail:open', 'true')
  })
  await page.route('**/api/features', route =>
    route.fulfill({ json: { evals: { enabled: false } } })
  )
  await page.route('**/rpc', async route => {
    const { id, method, params = {} } = route.request().postDataJSON()
    requests.push({ method, params })
    const result = (() => {
      if (method === 'getImportConfig') return { otlpHttpPort: 4318 }
      if (method === 'getStats')
        return {
          traces: {
            traceCount: 1,
            spanCount: 100,
            serviceCount: 1,
            errorCount: 0,
            lastReceived: null,
          },
          logs: { logCount: 30, errorCount: 0, lastReceived: null },
          metrics: { metricCount: 1, dataPointCount: 90, lastReceived: null },
          storage: { sizeBytes: 4096, maxSizeBytes: 0 },
          rejections: [],
        }
      if (method === 'discoverTelemetry') {
        const options = params.options
        if (options.kind === 'connections')
          return { rows: [], truncated: false }
        if (options.kind === 'services')
          return {
            rows: [
              [
                {
                  serviceName: 'checkout',
                  serviceNamespace: '7',
                  serviceIdentity,
                  serviceNamespaceKind: 'int64',
                  traceCount: 1,
                  spanCount: 100,
                  errorSpanCount: 0,
                  logCount: 30,
                  errorLogCount: 0,
                  logSeverityCounts: {},
                  metricCount: 1,
                  dataPointCount: 90,
                  exemplarCount: 0,
                  lastSeen: null,
                },
              ],
            ],
            truncated: false,
          }
        return { rows: [], truncated: false }
      }
      if (method === 'searchTraceSummaries')
        return [
          {
            traceID,
            hasRootSpan: true,
            rootSpan: { name: 'checkout request', serviceName: 'checkout' },
            startTime: '1792692000000000000',
            durationNs: '10000000',
            spanCount: 100,
            errorCount: 0,
            matchedSpans: [spanID, otherSpanID].map(spanID => ({
              traceID,
              spanID,
            })),
          },
        ]
      if (method === 'searchLogSummaries')
        return Array.from({ length: 30 }, (_, index) => ({
          logRef: `log-${index}`,
          serviceName: 'checkout',
          bodyPreview: `Matching log ${index}`,
          severityNumber: 9,
          severityText: 'INFO',
          timestamp: '1792692000000000000',
        }))
      if (method === 'searchMetricSummaries')
        return [
          {
            metricRef: 'metric-1',
            name: 'checkout.requests',
            serviceName: 'checkout',
            description: '',
            metricType: 'Gauge',
            seriesCount: 1,
            seriesCardinality: 1,
            dataPointCount: 90,
            lastValue: 7,
            lastSeen: '1792692000000000000',
          },
        ]
      return []
    })()
    await route.fulfill({ json: { jsonrpc: '2.0', id, result } })
  })
  await page.goto('/?time=all&q=needle')
  const details = page.getByRole('region', { name: 'Home details' })
  const foundOn = details.locator('[data-detail-section="found-on"]')
  const guide = page.getByRole('region', { name: 'Home sections' })
  await expect(
    guide.locator('[data-detail-section="storage"]')
  ).toHaveAttribute('data-detail-group', 'closed')
  await expect(foundOn.locator('summary')).toHaveText('Telemetry')
  const signals = foundOn.getByRole('tablist', { name: 'Telemetry signals' })
  const tracesChoice = signals.getByRole('tab', { name: /^Traces/ })
  const metricsChoice = signals.getByRole('tab', { name: /^Metrics/ })
  const logsChoice = signals.getByRole('tab', { name: /^Logs/ })
  async function selectSignal(signal: string) {
    await signals
      .getByRole('tab', { name: new RegExp(`^${signal}`, 'i') })
      .click()
  }
  const traces = foundOn.getByRole('region', { name: 'Matching traces' })
  const metrics = foundOn.getByRole('region', { name: 'Matching metrics' })
  const logs = foundOn.getByRole('region', { name: 'Matching logs' })
  const navigation = foundOn.getByRole('group', { name: 'page navigation' })
  await expect(tracesChoice).toHaveAccessibleName(/^Traces \[1\]/)
  await expect(metricsChoice).toHaveAccessibleName(/^Metrics \[1\]/)
  await expect(logsChoice).toHaveAccessibleName(/^Logs \[30\]/)
  const headerBox = (await foundOn.locator('summary').boundingBox())!
  const filterBox = (await signals.boundingBox())!
  expect(filterBox.x).toBeGreaterThan(headerBox.x)
  expect(filterBox.y).toBeGreaterThanOrEqual(headerBox.y)
  expect(filterBox.y + filterBox.height).toBeLessThanOrEqual(
    headerBox.y + headerBox.height + 1
  )
  await expect(foundOn.getByText('Found on', { exact: true })).toHaveCount(0)
  await expect(traces.getByRole('link')).toHaveCount(1)
  await expect(
    foundOn.getByRole('combobox', { name: 'Telemetry per page' })
  ).toHaveValue('25')
  await tracesChoice.focus()
  await expect(tracesChoice).toBeFocused()
  await tracesChoice.press('ArrowRight')
  await expect(metricsChoice).toBeFocused()
  await expect(metricsChoice).toHaveAttribute('aria-selected', 'true')
  await expect(metrics.getByRole('link')).toHaveCount(1)
  await expect(navigation).toContainText('1 / 1 page')
  await metricsChoice.press('End')
  await expect(logsChoice).toBeFocused()
  await expect(logsChoice).toHaveAttribute('aria-selected', 'true')
  await expect(logs.getByRole('link')).toHaveCount(25)
  await expect(logs.locator('.signal-row__description')).toHaveText(
    Array.from({ length: 25 }, (_, index) => `Matching log ${index}`)
  )
  await expect(navigation).toContainText('1 / 2 pages')
  await expect(
    navigation.getByRole('button', { name: 'Previous page' })
  ).toBeDisabled()
  const footerBox = (await navigation.boundingBox())!
  const filterBeforeScroll = (await signals.boundingBox())!
  await logs.evaluate(element => {
    element.scrollTop = 120
  })
  await expect.poll(() => logs.evaluate(element => element.scrollTop)).toBe(120)
  expect((await signals.boundingBox())!.y).toBeCloseTo(filterBeforeScroll.y, 0)
  expect((await navigation.boundingBox())!.y).toBeCloseTo(footerBox.y, 0)
  await selectSignal('metrics')
  await expect(navigation).toContainText('1 / 1 page')
  await selectSignal('logs')
  await expect.poll(() => logs.evaluate(element => element.scrollTop)).toBe(120)
  await navigation.getByRole('button', { name: 'Next page' }).click()
  await expect(logs.getByRole('link')).toHaveCount(5)
  await expect(logs.locator('.signal-row__description')).toHaveText(
    Array.from({ length: 5 }, (_, index) => `Matching log ${index + 25}`)
  )
  await expect(navigation).toContainText('2 / 2 pages')
  await expect(
    navigation.getByRole('button', { name: 'Next page' })
  ).toBeDisabled()
  await expect.poll(() => logs.evaluate(element => element.scrollTop)).toBe(0)
  await logsChoice.focus()
  await expect(logsChoice).toBeFocused()
  await logsChoice.press('Home')
  await expect(tracesChoice).toBeFocused()
  await expect(tracesChoice).toHaveAttribute('aria-selected', 'true')
  await expect(navigation).toContainText('1 / 1 page')
  await tracesChoice.press('End')
  await expect(navigation).toContainText('2 / 2 pages')
  await expect(logs.getByRole('link')).toHaveCount(5)
  const pageSize = foundOn.getByRole('combobox', { name: 'Telemetry per page' })
  await pageSize.selectOption('50')
  await expect(logs.getByRole('link')).toHaveCount(30)
  await expect(navigation).toContainText('1 / 1 page')
  await pageSize.selectOption('100')
  await expect(logs.getByRole('link')).toHaveCount(30)
  expect(
    await page.evaluate(() => localStorage.getItem('home-telemetry-page-size'))
  ).toBe('100')
  await pageSize.selectOption('25')
  await expect(details.locator('[data-detail-section]')).toHaveCount(1)
  expect(
    await details
      .locator('[data-detail-section]')
      .evaluateAll(elements =>
        elements.map(element => element.getAttribute('data-detail-section'))
      )
  ).toEqual(['found-on'])
  expect(
    await guide
      .locator('[data-detail-section]')
      .evaluateAll(elements =>
        elements.map(element => element.getAttribute('data-detail-section'))
      )
  ).toEqual(['guide', 'storage', 'issues'])
  await expect(
    guide.locator('[data-detail-section="storage"]')
  ).toHaveAttribute('data-detail-group', 'closed')
  await selectSignal('traces')
  const link = traces.getByRole('link')
  const href = new URL((await link.getAttribute('href'))!, page.url())
  expect(href.pathname).toBe(`/traces/${traceID}`)
  expect(href.searchParams.get('span')).toBe(spanID)
  expect(href.searchParams.get('q')).toBeNull()
  expect(href.searchParams.get('time')).toBe('all')
  const queries = requests.filter(
    request =>
      request.method.startsWith('search') &&
      request.method.endsWith('Summaries')
  )
  for (const request of queries) {
    expect(JSON.stringify(request.params.query)).toContain('needle')
    expect(JSON.stringify(request.params.query)).toContain(
      JSON.stringify(serviceIdentity).slice(1, -1)
    )
    expect(request.params.limit).toBeUndefined()
  }
  await expect(logsChoice).toContainText('[30]')
  await selectSignal('logs')
  await expect(navigation).toContainText('1 / 2 pages')
  await navigation.getByRole('button', { name: 'Last page' }).click()
  await expect(logs.getByRole('link')).toHaveCount(5)
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(navigation).toContainText('1 / 2 pages')
  await expect(logs.getByRole('link')).toHaveCount(25)
  await expect(logs.locator('.signal-row__description').first()).toHaveText(
    'Matching log 0'
  )
  await expect(
    guide.locator('[data-detail-section="storage"]')
  ).toHaveAttribute('data-detail-group', 'closed')
  expect(errors).toEqual([])
})
