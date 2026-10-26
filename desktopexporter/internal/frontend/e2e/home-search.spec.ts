import { expect, test } from '@playwright/test'

test('Home selection, map, telemetry and workspace links share the submitted query', async ({
  page,
}) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  const service = {
    serviceName: 'checkout',
    serviceNamespace: '7',
    serviceNameKind: 'string',
    serviceNamespaceKind: 'string',
    serviceIdentity: JSON.stringify([
      { kind: 'string', value: 'checkout' },
      { kind: 'string', value: '7' },
    ]),
    traceCount: 2,
    spanCount: 3,
    errorSpanCount: 0,
    logCount: 4,
    errorLogCount: 0,
    logSeverityCounts: {},
    metricCount: 1,
    dataPointCount: 8,
    exemplarCount: 0,
    lastSeen: null,
  }
  const discovery: Record<string, unknown>[] = []
  const metricQueries: unknown[] = []
  const logQueries: Record<string, unknown>[] = []
  const logPageQueries: Record<string, unknown>[] = []
  await page.addInitScript(() => {
    localStorage.setItem('signal-detail:open', 'true')
  })
  await page.route('**/api/features', route =>
    route.fulfill({ json: { evals: { enabled: false } } })
  )
  await page.route('**/rpc', async route => {
    const { id, method, params } = route.request().postDataJSON()
    if (method === 'searchMetricSummaries') metricQueries.push(params)
    if (method === 'searchLogSummaries') logQueries.push(params)
    if (method === 'searchLogPage') logPageQueries.push(params)
    const result = (() => {
      if (method === 'getStats')
        return {
          traces: {
            traceCount: 9,
            spanCount: 12,
            serviceCount: 2,
            errorCount: 0,
            lastReceived: null,
          },
          logs: { logCount: 10, errorCount: 0, lastReceived: null },
          metrics: { metricCount: 5, dataPointCount: 20, lastReceived: null },
          rejections: [],
        }
      if (method === 'getImportConfig') return { otlpHttpPort: 4318 }
      if (method === 'searchLogPage') return { total: 0, logs: [] }
      if (method === 'searchLogSummaries')
        return Array.from(
          { length: JSON.stringify(params.query).includes('declined') ? 1 : 4 },
          (_, index) => ({
            logRef: `checkout-log-${index}`,
            serviceName: 'checkout',
            bodyPreview: `Checkout log ${index}`,
            severityNumber: 9,
            severityText: 'INFO',
            timestamp: '1792692000000000000',
          })
        )
      if (method === 'discoverTelemetry') {
        const options = params.options
        discovery.push(options)
        if (options.kind === 'services') {
          const value = options.queries?.logs?.query?.value
          const items =
            value === 'missing'
              ? []
              : value
                ? [
                    {
                      ...service,
                      traceCount: 0,
                      spanCount: 0,
                      metricCount: 0,
                      dataPointCount: 0,
                      logCount: 1,
                    },
                  ]
                : [
                    service,
                    {
                      ...service,
                      serviceName: 'worker',
                      serviceIdentity: JSON.stringify([
                        { kind: 'string', value: 'worker' },
                        { kind: 'string', value: '7' },
                      ]),
                    },
                    {
                      ...service,
                      serviceNamespaceKind: 'int64',
                      serviceIdentity: JSON.stringify([
                        { kind: 'string', value: 'checkout' },
                        { kind: 'int64', value: '7' },
                      ]),
                    },
                  ]
          return {
            rows: items.map(item => [item]),
            truncated: false,
            totals: {
              home: items.length,
              traces: value ? 0 : 3,
              metrics: value ? 0 : 2,
              logs: value === 'missing' ? 0 : value ? 1 : 8,
            },
          }
        } else return { rows: [], truncated: false }
      }
      return []
    })()
    await route.fulfill({ json: { jsonrpc: '2.0', id, result } })
  })
  await page.goto('/?time=all')
  const serviceCards = page.locator('.service-map__node .signal-row')
  await expect(serviceCards).toHaveCount(3)
  const typedService = page
    .locator('.service-map__node')
    .getByRole('button', { name: /^checkout.*int64/ })
  await typedService.focus()
  await typedService.press('Enter')
  await expect(typedService).toHaveAttribute('aria-pressed', 'true')
  const typedIdentity = JSON.stringify([
    { kind: 'string', value: 'checkout' },
    { kind: 'int64', value: '7' },
  ])
  await expect
    .poll(() =>
      discovery.some(
        options =>
          options.kind === 'connections' &&
          options.serviceIdentity === typedIdentity
      )
    )
    .toBe(true)
  const stringService = page
    .locator('.service-map__node')
    .getByRole('button', { name: /^checkout/ })
    .filter({ hasNotText: 'int64' })
  await stringService.focus()
  await stringService.press('Space')
  await expect(stringService).toHaveAttribute('aria-pressed', 'true')
  await expect(typedService).toHaveAttribute('aria-pressed', 'false')
  const input = page.getByRole('searchbox', { name: 'Search telemetry' })
  await expect(input).toHaveAttribute('placeholder', 'Search telemetry…')
  await input.fill('declined')
  await input.press('Enter')
  await expect(serviceCards).toHaveCount(1)
  await expect(
    page.getByRole('link', { name: /^Logs(?: \[|$)/ })
  ).toHaveAccessibleDescription('Matching logs: [1]')
  const traces = page
    .getByRole('navigation', { name: 'Primary' })
    .getByLabel(/^Traces(?: \[|$)/)
  await expect(traces).toHaveAttribute('aria-disabled', 'true')
  await expect(traces).not.toHaveAttribute('href')
  const telemetry = page.getByRole('tablist', { name: 'Telemetry signals' })
  await expect(telemetry.getByRole('tab', { name: /^Traces/ })).toBeDisabled()
  await expect(telemetry.getByRole('tab', { name: /^Metrics/ })).toBeDisabled()
  const logsChoice = telemetry.getByRole('tab', {
    name: /^Logs \[1\]/,
  })
  await logsChoice.focus()
  await expect(logsChoice).toBeFocused()
  await logsChoice.press('Home')
  await expect(logsChoice).toBeFocused()
  await expect(logsChoice).toHaveAttribute('aria-selected', 'true')
  await expect(serviceCards.getByRole('link')).toHaveCount(0)
  await input.fill('an unsubmitted draft')
  // Home can fetch in the same millisecond as the picker writes its URL.
  // Keep those anchors equal, then advance the clock before navigation so a
  // destination that incorrectly retains the live preset cannot pass by chance.
  const presetTime = new Date('2026-10-25T12:00:00.123Z')
  await page.clock.setFixedTime(presetTime)
  await page.getByRole('button', { name: /^Change time range,/ }).click()
  await page.getByRole('button', { name: 'Last 5 min', exact: true }).click()
  await expect(page).toHaveURL(/start=/)
  await page.keyboard.press('Escape')
  await expect(input).toHaveValue('an unsubmitted draft')
  expect(new URL(page.url()).searchParams.get('q')).toBe('declined')
  await expect
    .poll(() =>
      discovery.some(
        options =>
          options.kind === 'connections' &&
          options.serviceIdentity === service.serviceIdentity &&
          JSON.stringify(options.queries).includes('declined')
      )
    )
    .toBe(true)
  await expect
    .poll(() =>
      logQueries.some(params => {
        const query = JSON.stringify(params.query)
        return (
          query.includes('declined') &&
          query.includes(
            JSON.stringify(service.serviceIdentity).slice(1, -1)
          ) &&
          !query.includes('an unsubmitted draft') &&
          params.startTime != null &&
          params.endTime != null
        )
      })
    )
    .toBe(true)
  // Navigation must carry the exact received reading into the next workspace,
  // even when it equals the picker's URL snapshot.
  const reading = [...discovery]
    .reverse()
    .find(options => options.kind === 'services' && options.startTime != null)!
  const receivedRange = {
    startTime: reading.startTime,
    endTime: reading.endTime,
  }
  expect(
    BigInt(String(reading.endTime)) - BigInt(String(reading.startTime))
  ).toBe(300_000_000_000n)
  for (const options of discovery.filter(
    options => options.startTime != null
  )) {
    expect(options).toMatchObject(receivedRange)
  }
  for (const params of logQueries.filter(params => params.startTime != null)) {
    expect(params).toMatchObject(receivedRange)
  }
  const link = page
    .getByRole('navigation', { name: 'Primary' })
    .getByRole('link', { name: /^Logs(?: \[|$)/ })
  const target = new URL((await link.getAttribute('href'))!, page.url())
  expect(target.searchParams.get('q')).toBe('search("declined")')
  expect(target.searchParams.get('start')).toBe(
    String(BigInt(String(reading.startTime)) / 1_000_000n)
  )
  expect(target.searchParams.get('end')).toBe(
    String(BigInt(String(reading.endTime)) / 1_000_000n)
  )
  expect(target.searchParams.get('end')).toBe(String(presetTime.getTime()))
  expect(new URL(page.url()).searchParams.get('end')).toBe(
    target.searchParams.get('end')
  )
  await page.clock.setFixedTime(new Date(presetTime.getTime() + 161))
  await link.click()
  await expect(page).toHaveURL(/\/logs\?q=/)
  await expect.poll(() => logPageQueries.length).toBeGreaterThan(0)
  for (const params of logPageQueries) {
    expect(params).toMatchObject(receivedRange)
  }
  expect(new URL(page.url()).searchParams.get('start')).toBe(
    target.searchParams.get('start')
  )
  expect(new URL(page.url()).searchParams.get('end')).toBe(
    target.searchParams.get('end')
  )
  await expect(
    page.getByRole('link', { name: /^Logs(?: \[|$)/ })
  ).toHaveAccessibleDescription('Stored logs: [10]')
  const pageReads = logPageQueries.length
  await page.reload()
  await expect.poll(() => logPageQueries.length).toBeGreaterThan(pageReads)
  for (const params of logPageQueries) {
    expect(params).toMatchObject(receivedRange)
  }
  await expect(page.getByRole('alert')).toHaveCount(0)
  expect(new URL(page.url()).searchParams.get('q')).toBe(
    target.searchParams.get('q')
  )
  await page.goBack()
  expect(new URL(page.url()).searchParams.get('q')).toBe('declined')
  await expect(serviceCards).toHaveCount(1)
  await input.fill('body !~ "healthcheck"')
  await input.press('Enter')
  await expect(link).toHaveAttribute('href', /healthcheck/)
  await link.click()
  expect(new URL(page.url()).searchParams.get('q')).toContain(
    'body !~ "healthcheck"'
  )
  await expect(page.getByRole('alert')).toHaveCount(0)
  // Workspace navigation carries time, but each signal keeps its own query.
  const logsQuery = new URL(page.url()).searchParams.get('q')
  await page.getByRole('link', { name: /^Metrics(?: \[|$)/ }).click()
  expect(new URL(page.url()).searchParams.get('q')).toBeNull()
  await expect(page.getByRole('alert')).toHaveCount(0)
  await page.getByRole('link', { name: /^Logs(?: \[|$)/ }).click()
  expect(new URL(page.url()).searchParams.get('q')).toBe(logsQuery)
  await page.goBack()
  await page.goBack()
  await page.goBack()
  await input.fill('missing')
  await input.press('Enter')
  await expect(serviceCards).toHaveCount(0)
  await expect(
    page.getByRole('link', { name: /^Home(?: \[|$)/ })
  ).toHaveAccessibleDescription('Matching services: [0]')
  await expect(
    page.getByText('No services match this search and time range.')
  ).toBeVisible()
  // A fresh metric URL submits before reactive parent notifications can run.
  metricQueries.length = 0
  const metricQuery = `serviceIdentity = ${JSON.stringify(service.serviceIdentity)}`
  await page.goto(`/metrics?time=all&q=${encodeURIComponent(metricQuery)}`)
  await expect
    .poll(() =>
      JSON.stringify(metricQueries).includes(
        JSON.stringify(service.serviceIdentity)
      )
    )
    .toBe(true)
  await expect(page.getByRole('alert')).toHaveCount(0)
  expect(errors).toEqual([])
})
