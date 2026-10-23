import { expect, test } from '@playwright/test'

test('unified attribute coverage opens exact matching summary cards and a formatted value above Storage', async ({
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
  const table = page.getByRole('table', { name: 'Attributes', exact: true })
  const heading = table.locator('tr[data-attribute-heading="payload"]')
  await expect(heading).toHaveCount(1)
  await expect(
    heading.getByRole('group', { name: 'Attribute value coverage' })
  ).toBeVisible()
  const rows = table.locator('tr[data-attribute-key="payload"]')
  await expect(rows.first()).toBeHidden()
  const details = page.getByRole('region', { name: 'Home details' })
  const foundOn = details.locator('[data-detail-section="found-on"]')
  const storage = details.locator('[data-detail-section="storage"]')
  await heading
    .getByRole('button', {
      name: 'Show payload · string · resource in details',
    })
    .click()
  await expect(
    foundOn.getByRole('region', { name: 'Matching traces' }).getByRole('link')
  ).toHaveCount(1)
  await expect(
    foundOn.getByRole('region', { name: 'Matching logs' }).getByRole('link')
  ).toHaveCount(25)
  await expect(
    foundOn.getByRole('region', { name: 'Matching metrics' }).getByRole('link')
  ).toHaveCount(1)
  await expect(foundOn).toContainText('2 matching spans')
  await expect(foundOn).toContainText('90 matching datapoints')
  await foundOn.getByRole('button', { name: 'More logs' }).click()
  await expect(
    foundOn.getByRole('region', { name: 'Matching logs' }).getByRole('link')
  ).toHaveCount(30)
  expect((await foundOn.boundingBox())!.y).toBeLessThan(
    (await storage.boundingBox())!.y
  )
  const tableNode = await table.elementHandle()
  const discoveryReads = requests.filter(
    request => request.method === 'discoverTelemetry'
  ).length
  await heading
    .getByRole('button', { name: 'Expand payload · resource values' })
    .click()
  await expect(rows.first().locator('td:nth-child(3)')).toHaveText('[50%]')
  await expect(rows.last()).toContainText('rare')
  await rows
    .first()
    .getByRole('button', {
      name: 'Show resource attribute: payload in details',
    })
    .click()
  const valuePane = details.locator('[data-detail-section="value"]')
  await expect(valuePane.getByLabel('JSON value of payload')).toBeVisible()
  await expect(valuePane).toContainText('90 spans · 1 logs · 9 datapoints')
  await expect(
    foundOn.getByRole('region', { name: 'Matching logs' }).getByRole('link')
  ).toHaveCount(1)
  await expect(foundOn).toContainText('1 matching span')
  await expect(foundOn).toContainText('9 matching datapoints')
  expect((await valuePane.boundingBox())!.y).toBeLessThan(
    (await foundOn.boundingBox())!.y
  )
  expect((await foundOn.boundingBox())!.y).toBeLessThan(
    (await storage.boundingBox())!.y
  )
  await valuePane.getByRole('group', { name: 'payload', exact: true }).hover()
  await valuePane
    .getByRole('button', { name: 'Copy payload (resource)' })
    .click()
  expect(
    await page.evaluate(
      () => (window as Window & { copiedValue?: string }).copiedValue
    )
  ).toBe(value)
  const link = foundOn
    .getByRole('region', { name: 'Matching traces' })
    .getByRole('link')
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
  expect(errors).toEqual([])
})
