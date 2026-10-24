import { expect, test } from '@playwright/test'

test('attribute value inspection scopes counted Telemetry tabs with independent 25-card pages', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1920, height: 1100 })
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  const value =
    '{"result":{"id":9007199254740993,"message":"it\'s exact"},"nested":[{"ready":true}]}'
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
    Object.defineProperty(navigator, 'clipboard', {
      value: {
        writeText: async (text: string) => {
          Object.assign(window, { copiedValue: text })
        },
      },
    })
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
        const foundOn = [
          { signal: options.signal, ownerType: options.ownerType },
        ]
        if (options.kind === 'keys')
          return {
            rows:
              options.ownerType === 'resource'
                ? [[{ key: 'payload', kind: 'string', foundOn }]]
                : [],
            truncated: false,
          }
        const counts =
          options.signal === 'traces'
            ? [90, 9, 1]
            : options.signal === 'logs'
              ? [1, 8, 1]
              : [9, 80, 1]
        const denominator = counts.reduce(
          (sum: number, count: number) => sum + count,
          0
        )
        return {
          rows: [value, 'second', 'rare'].map((text, index) => [
            {
              value: { kind: 'string', value: text },
              foundOn,
              count: counts[index],
              denominator,
              relativeFrequency: counts[index] / denominator,
            },
          ]),
          truncated: false,
        }
      }
      if (method === 'query') {
        const narrowed = params.sql.includes('AND a.value = ')
        if (params.sql.includes('FROM spans s'))
          return {
            rows: [
              [
                traceID,
                narrowed ? [spanID] : [spanID, otherSpanID],
                narrowed ? 1 : 2,
              ],
            ],
            truncated: false,
          }
        if (params.sql.includes('FROM logs l'))
          return {
            rows: Array.from({ length: narrowed ? 1 : 30 }, (_, index) => [
              `log-${index}`,
              [],
              1,
            ]),
            truncated: false,
          }
        return { rows: [['metric-1', [], narrowed ? 9 : 90]], truncated: false }
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
  const table = details.getByRole('table', { name: 'Attributes', exact: true })
  const heading = table.locator('tr[data-attribute-heading="payload"]')
  await expect(heading).toHaveCount(1)
  await expect(heading.locator('.attribute-frequency-bar')).toHaveCount(0)
  const rows = table.locator('tr[data-attribute-key="payload"]')
  await expect(rows.first()).toBeHidden()
  const foundOn = details.locator('[data-detail-section="found-on"]')
  const attributes = details.locator('[data-detail-section="attributes"]')
  const guide = page.getByRole('region', { name: 'Getting started sections' })
  await expect(
    guide.locator('[data-detail-section="storage"]')
  ).toHaveAttribute('data-detail-group', 'closed')
  await expect(foundOn.locator('summary')).toHaveText('Telemetry')
  const tabs = foundOn.getByRole('tablist', { name: 'Telemetry signals' })
  const tracesTab = tabs.getByRole('tab', { name: /^Traces/ })
  const metricsTab = tabs.getByRole('tab', { name: /^Metrics/ })
  const logsTab = tabs.getByRole('tab', { name: /^Logs/ })
  const traces = foundOn.getByRole('region', { name: 'Matching traces' })
  const metrics = foundOn.getByRole('region', { name: 'Matching metrics' })
  const logs = foundOn.getByRole('region', { name: 'Matching logs' })
  const navigation = foundOn.getByRole('group', { name: 'page navigation' })
  await expect(tabs.getByRole('tab').nth(0)).toHaveAccessibleName(
    'Traces [1] traces'
  )
  await expect(tabs.getByRole('tab').nth(1)).toHaveAccessibleName(
    'Metrics [1] metrics'
  )
  await expect(tabs.getByRole('tab').nth(2)).toHaveAccessibleName(
    'Logs [30] logs'
  )
  const headerBox = (await foundOn.locator('summary').boundingBox())!
  const tabBox = (await tabs.boundingBox())!
  expect(tabBox.x).toBeGreaterThan(headerBox.x)
  expect(tabBox.y).toBeGreaterThanOrEqual(headerBox.y)
  expect(tabBox.y + tabBox.height).toBeLessThanOrEqual(
    headerBox.y + headerBox.height + 1
  )
  await heading
    .getByRole('button', {
      name: 'Show payload · string · resource in details',
    })
    .click()
  await expect(foundOn.locator('summary')).toContainText('payload')
  await expect(foundOn.locator('summary')).toContainText('string')
  await expect(foundOn.locator('summary')).toContainText('resource')
  await expect(foundOn.getByText('Found on', { exact: true })).toHaveCount(0)
  await expect(traces.getByRole('link')).toHaveCount(1)
  await expect(traces).toContainText('2 matching spans')
  await expect(foundOn.getByText('25 per page', { exact: true })).toBeVisible()
  await tracesTab.focus()
  await tracesTab.press('ArrowRight')
  await expect(metricsTab).toBeFocused()
  await expect(metricsTab).toHaveAttribute('aria-selected', 'true')
  await expect(metrics.getByRole('link')).toHaveCount(1)
  await expect(metrics).toContainText('90 matching datapoints')
  await expect(navigation).toContainText('1 / 1 page')
  await metricsTab.press('End')
  await expect(logsTab).toBeFocused()
  await expect(logsTab).toHaveAttribute('aria-selected', 'true')
  await expect(logs.getByRole('link')).toHaveCount(25)
  await expect(logs.locator('.signal-row__description')).toHaveText(
    Array.from({ length: 25 }, (_, index) => `Matching log ${index}`)
  )
  await expect(navigation).toContainText('1 / 2 pages')
  await expect(
    navigation.getByRole('button', { name: 'Previous page' })
  ).toBeDisabled()
  const footerBox = (await navigation.boundingBox())!
  const tabsBeforeScroll = (await tabs.boundingBox())!
  await logs.evaluate(element => {
    element.scrollTop = 120
  })
  await expect.poll(() => logs.evaluate(element => element.scrollTop)).toBe(120)
  expect((await tabs.boundingBox())!.y).toBeCloseTo(tabsBeforeScroll.y, 0)
  expect((await navigation.boundingBox())!.y).toBeCloseTo(footerBox.y, 0)
  await logsTab.press('ArrowLeft')
  await expect(metricsTab).toBeFocused()
  await expect(navigation).toContainText('1 / 1 page')
  await metricsTab.press('ArrowRight')
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
  await logsTab.focus()
  await logsTab.press('Home')
  await expect(tracesTab).toBeFocused()
  await expect(tracesTab).toHaveAttribute('aria-selected', 'true')
  await expect(navigation).toContainText('1 / 1 page')
  await tracesTab.press('End')
  await expect(navigation).toContainText('2 / 2 pages')
  await expect(logs.getByRole('link')).toHaveCount(5)
  expect((await foundOn.boundingBox())!.y).toBeLessThan(
    (await attributes.boundingBox())!.y
  )
  const tableNode = await table.elementHandle()
  const discoveryReads = requests.filter(
    request => request.method === 'discoverTelemetry'
  ).length
  await heading
    .getByRole('button', { name: 'Expand payload · resource values' })
    .click()
  await table.getByRole('button', { name: 'Sort by percentage' }).click()
  await expect(rows.first().locator('td:nth-child(2)')).toHaveText('100')
  await expect(rows.first().locator('td:nth-child(3)')).toHaveText('[50%]')
  await expect(rows.locator('.attribute-frequency-bar')).toHaveCount(3)
  await expect(rows.first().locator('.attribute-frequency-bar')).toBeVisible()
  await expect(rows.last()).toContainText('rare')
  const inspect = rows.first().getByRole('button', {
    name: 'Show resource attribute: payload in details',
  })
  await expect(inspect).toHaveAttribute('title', value)
  await inspect.focus()
  await inspect.press('Enter')
  await expect(rows.first()).toHaveClass(/table-row--selected/)
  await expect(logsTab).toHaveAccessibleName('Logs [1] logs')
  await expect(logs.getByRole('link')).toHaveCount(1)
  await expect(logs.locator('.signal-row__description')).toHaveText(
    'Matching log 0'
  )
  await expect(navigation).toContainText('1 / 1 page')
  await tracesTab.click()
  await expect(traces).toContainText('1 matching span')
  await metricsTab.click()
  await expect(metrics).toContainText('9 matching datapoints')
  expect((await foundOn.boundingBox())!.y).toBeLessThan(
    (await attributes.boundingBox())!.y
  )
  await expect(details.locator('[data-detail-section]')).toHaveCount(2)
  expect(
    await details
      .locator('[data-detail-section]')
      .evaluateAll(elements =>
        elements.map(element => element.getAttribute('data-detail-section'))
      )
  ).toEqual(['found-on', 'attributes'])
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
  const summaryReads = requests.filter(request =>
    request.method.endsWith('Summaries')
  ).length
  await rows.first().getByRole('button', { name: 'Copy payload value' }).click()
  expect(
    await page.evaluate(
      () => (window as Window & { copiedValue?: string }).copiedValue
    )
  ).toBe(value)
  expect(
    requests.filter(request => request.method.endsWith('Summaries'))
  ).toHaveLength(summaryReads)
  await tracesTab.click()
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
  expect(
    requests.some(
      request =>
        request.method === 'query' &&
        String(request.params.sql).includes(
          JSON.stringify({ kind: 'string', value }).replaceAll("'", "''")
        )
    )
  ).toBe(true)
  expect(
    requests.filter(request => request.method === 'discoverTelemetry')
  ).toHaveLength(discoveryReads)
  expect(await tableNode!.evaluate(node => node.isConnected)).toBe(true)
  // Returning to the key clears the exact-value filter and resets each page.
  await heading
    .getByRole('button', {
      name: 'Show payload · string · resource in details',
    })
    .click()
  await expect(logsTab).toHaveAccessibleName('Logs [30] logs')
  await logsTab.click()
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
